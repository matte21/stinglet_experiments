package main

import (
	"bytes"
	"database/sql"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/coreos/go-systemd/v22/sdjournal"
)

// CLI flags.
var (
	resultsDir            string
	resultsFilePrefix     string
	traceDB               string
	traceMachineID        int
	cxlPercentageDBColumn string
)

var (
	admittedPodsCount            uint32
	admitTimeSamplesCount        atomic.Uint32
	admitTimeSamplesWrittenCount uint32
)

type kubeletLogLineHandler struct {
	admitTimeSamplesFile *os.File
	admitTimeRegexp      *regexp.Regexp
}

func main() {
	parseFlagsOrDie()

	f := openAndInitAdmitTimesFileOrDie()
	defer func() {
		if err := f.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to close kubelet admit times result file %s: %v\n", f.Name(), err)
		}
	}()

	// Start collecting pod admission times from kubelet logs, in the background.
	stop := make(chan time.Time)
	defer func() {
		// Stop the samples collector only after the samples for all pods have been collected.
		for admitTimeSamplesCount.Load() < admittedPodsCount {
			// Give kubelet some time to emit log lines with the admit times of the last pods.
			time.Sleep(100 * time.Millisecond)
		}
		close(stop)

		lostSamplesCount := admittedPodsCount - admitTimeSamplesWrittenCount
		if lostSamplesCount > 0 {
			fmt.Fprintf(os.Stderr, "Warning: %d admit time samples out of %d could not be written to file %s",
				lostSamplesCount, admittedPodsCount, f.Name())
		}
	}()
	startKubeletAdmitTimeCollectionOrDie(stop, &kubeletLogLineHandler{
		admitTimeSamplesFile: f,
		admitTimeRegexp:      regexp.MustCompile(`time="([^"]+)"`),
	})

	// play trace
	playTrace()
}

// "Die", i.e., exit immediately, if the passed flags are invalid.
func parseFlagsOrDie() {
	flag.StringVar(&resultsDir, "results-dir", "", "Path to the directory where results are stored")
	flag.StringVar(&resultsFilePrefix, "results-file-prefix", "", "Prefix for result files names")
	flag.StringVar(&traceDB, "trace-db", "", "Path to the SQLite trace database file")
	flag.IntVar(&traceMachineID, "trace-machine-id", -1, "Azure machine type ID the resource requests in the trace are relative to")
	flag.StringVar(&cxlPercentageDBColumn, "cxl-percentage-db-column", "", "Name of the column in the trace table that stores the percentage of working set memory that the VM wants on CXL")

	flag.Parse()

	resultsDir = strings.TrimSpace(resultsDir)
	resultsFilePrefix = strings.TrimSpace(resultsFilePrefix)
	traceDB = strings.TrimSpace(traceDB)
	cxlPercentageDBColumn = strings.ToLower(strings.TrimSpace(cxlPercentageDBColumn))

	if resultsDir == "" {
		fmt.Fprintln(os.Stderr, "Fatal: --results-dir is required and must be non-empty.")
		flag.Usage()
		os.Exit(1)
	}
	if resultsFilePrefix == "" {
		fmt.Fprintln(os.Stderr, "Fatal: --results-file-prefix is required and must be non-empty.")
		flag.Usage()
		os.Exit(1)
	}
	if traceDB == "" {
		fmt.Fprintln(os.Stderr, "Fatal: --trace-db is required and must be non-empty.")
		flag.Usage()
		os.Exit(1)
	}
	if traceMachineID == -1 {
		fmt.Fprintln(os.Stderr, "Fatal: --trace-machine-id is required")
		flag.Usage()
		os.Exit(1)
	}
	if cxlPercentageDBColumn == "" {
		fmt.Fprintln(os.Stderr, "Fatal: --cxl-percentage-db-column is required, and must be one of greensku, pond, and nop.")
		flag.Usage()
		os.Exit(1)
	}
	if cxlPercentageDBColumn != "pond" && cxlPercentageDBColumn != "greensku" && cxlPercentageDBColumn != "nop" {
		fmt.Fprintf(os.Stderr, "Fatal: --cxl-percentage-db-column has value %s; must be one of: pond, greensku, nop.\n", cxlPercentageDBColumn)
		flag.Usage()
		os.Exit(1)
	}
}

func openAndInitAdmitTimesFileOrDie() *os.File {
	path := filepath.Join(resultsDir, resultsFilePrefix+"admit_times.csv")
	f, err := os.Create(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Fatal: failed to create admit times file %s: %v\n", path, err)
		os.Exit(1)
	}

	if _, err := fmt.Fprintln(f, ",admit time usecs,,"); err != nil {
		fmt.Fprintf(os.Stderr, "Fatal: failed to write header to admit times file %s: %v\n", path, err)
		os.Exit(1)
	}

	return f
}

func startKubeletAdmitTimeCollectionOrDie(stop <-chan time.Time, w io.Writer) {
	cfg := sdjournal.JournalReaderConfig{
		Matches: []sdjournal.Match{
			{
				Field: sdjournal.SD_JOURNAL_FIELD_SYSTEMD_UNIT,
				Value: "kubelet.service",
			},
		},
		// Only get new logs from now on.
		Since: time.Duration(0),
	}

	jr, err := sdjournal.NewJournalReader(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Fatal: failed to open systemd journal to collect kubelet admission times: %v\n", err)
		os.Exit(1)
	}

	go func() {
		defer func() {
			if err := jr.Close(); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to close systemd journal streamer (for collecting kubelet admission times): %v", err)
			} else {
				fmt.Println("Closed systemd journal streamer (for collecting kubelet admission times)")
			}
		}()
		if err := jr.Follow(stop, w); err != nil && err != sdjournal.ErrExpired {
			fmt.Fprintf(os.Stderr, "Error while following systemd journal collecting kubelet admission times: %v\n", err)
		}
	}()
}

// example kubelet log line that this function matches and parses:
// Dec 09 14:43:19 vxf01 kubelet[1452415]: I1209 14:43:19.025926 1452415 topology_manager.go:232] "admit time" time="5.463692ms"
func (h *kubeletLogLineHandler) Write(l []byte) (int, error) {
	if bytes.Contains(l, []byte("admit time")) {
		admitTime := h.admitTimeRegexp.FindStringSubmatch(string(l))
		if len(admitTime) >= 2 {
			// Increase even if the write is unsuccessful, otherwise we have a deadlock.
			admitTimeSamplesCount.Add(1)

			d, err := time.ParseDuration(admitTime[1])
			if err != nil {
				return 0, fmt.Errorf("failed to parse admit time duration %s: %w", admitTime[1], err)
			}
			if _, err := fmt.Fprintf(h.admitTimeSamplesFile, ",%d,,\n", d.Microseconds()); err != nil {
				return 0, fmt.Errorf("failed to write admit time to %s: %w", h.admitTimeSamplesFile.Name(), err)
			}

			admitTimeSamplesWrittenCount++
		}
	}
	return len(l), nil
}

func playTrace() error {
	// Connect to the DB storing the trace.
	db, err := sql.Open("sqlite", traceDB)
	if err != nil {
		return fmt.Errorf("Fatal: failed to open trace DB %s: %w", traceDB, err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to close trace DB %s: %v\n", traceDB, err)
		}
	}()

	// Query the table storing the trace we care about.
	var query string
	if cxlPercentageDBColumn == "nop" {
		query = fmt.Sprintf(`SELECT priority, cores, memory FROM "events-machine-id-%d" ORDER BY time`, traceMachineID)
	} else {
		query = fmt.Sprintf(`SELECT priority, cores, memory, cxl_mem_percent_%s FROM "events-machine-id-%d" ORDER BY time`,
			cxlPercentageDBColumn, traceMachineID)
	}
	traceEvents, err := db.Query(query)
	if err != nil {
		return fmt.Errorf("failed to run query \"%s\" against trace DB %s: %w", query, traceDB, err)
	}
	defer func() {
		if err := traceEvents.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to close trace DB %s rows: %v\n", traceDB, err)
		}
	}()

	// todo: more stuff.
	// Now actually play the trace.
	for e := traceEvents.Next() {
		if err := rows.Scan(&priority, &cores, &memory, &cxlMemPercent); err != nil {
            return fmt.Errorf("failed to scan row: %w", err)
        }

	}
	// Iterate -> parse into Pod


	return nil
}
