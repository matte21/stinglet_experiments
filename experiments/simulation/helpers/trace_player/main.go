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

// All names are self-explanatory, except for ignoredPodsCount. That counts the pods that haven't
// been scheduled to our reference host because it didn't have enough free memory and CPUs.
var (
	ignoredPodsCount             uint32
	rejectedPodsCount            uint32
	admittedPodsCount            uint32
	admitTimeSamplesCount        atomic.Uint32
	admitTimeSamplesWrittenCount uint32
)

// totals are used to scale the VM/pods resource requests (which are relative to the host's capacity).
// frees are used to check whether a VM/pod fits on the host where stinglet/kubelet is running.
var (
	hostTotalBytes uint64
	hostFreeBytes  uint64

	hostTotalMilliCores uint64
	hostFreeMilliCores  uint64
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

	// Load the table storing the VMs/pods arrivals and departures from the SQLite DB where it's
	// stored.
	traceEvents, cleanupFunc, err := getTrace()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Fatal: failed to get trace of VMs/pods arrivals and departures: %s.", err)
		// return instead of os.Exit so that deferred functions still execute.
		return
	}
	defer cleanupFunc()

	playTrace(traceEvents)
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

// Invoke cleanup only if err == nil (you can invoke it even if err != nil, but it's a nop).
func getTrace() (trace *sql.Rows, cleanup func(), err error) {
	cleanup = func() {}

	db, err := sql.Open("sqlite", traceDB)
	if err != nil {
		err = fmt.Errorf("failed to open trace DB %s: %w", traceDB, err)
		return
	}
	defer func() {
		if err != nil {
			if err1 := db.Close(); err1 != nil {
				fmt.Fprintf(os.Stderr, "Failed to close trace DB %s: %v\n", traceDB, err1)
			}
		}
	}()

	var query string
	if cxlPercentageDBColumn == "nop" {
		query = fmt.Sprintf(`SELECT priority, cores, memory FROM "events-machine-id-%d" ORDER BY time`, traceMachineID)
	} else {
		query = fmt.Sprintf(`SELECT priority, cores, memory, cxl_mem_percent_%s FROM "events-machine-id-%d" ORDER BY time`,
			cxlPercentageDBColumn, traceMachineID)
	}
	trace, err = db.Query(query)
	if err != nil {
		err = fmt.Errorf("failed to run query \"%s\" against trace DB %s: %w", query, traceDB, err)
		return
	}

	cleanup = func() {
		if err := trace.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to close trace DB %s rows: %v\n", traceDB, err)
		}
		if err := db.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to close trace DB %s: %v\n", traceDB, err)
		}
	}

	return
}

func playTrace(traceEvents *sql.Rows) error {
	// collectively this variables describe a trace event.
	var priority int
	var cores float64
	var memory float64
	var cxlMemPercent sql.NullInt32

	var err error

	for traceEvents.Next() {
		// Read a single event, either a VM/Pod arrival/departure.
		if cxlPercentageDBColumn == "nop" {
			err = traceEvents.Scan(&priority, &cores, &memory)
		} else {
			err = traceEvents.Scan(&priority, &cores, &memory, &cxlMemPercent)
		}
		if err != nil {
			return fmt.Errorf("failed to scan sql row storing trace event: %w", err)
		}

		if eventIsArrival(cores) {
			handleArrival(priority, cores, memory, cxlMemPercent)
		} else {
			// process departure.
		}
	}

	return nil
}

func handleArrival(priority int, normalizedCores, normalizedMemory float64, cxlMemPercent sql.NullInt32) {
	requestedBytes := uint64(float64(hostTotalBytes) * normalizedMemory)
	requestedMilliCores := uint64(float64(hostTotalMilliCores) * normalizedCores)

	if requestedBytes > hostFreeBytes || requestedMilliCores > hostFreeMilliCores {
		ignoredPodsCount++
		return
	}

	// create file for it.
	// write file for it.
	// poll kubelet API for outcome -- need connection.
	// If rejected - increase reject counter and done.
	// If accepted.
	// Add to active set.
	// read kubelet files -- need connection.
	// update resource availability.
	// sample memory utilization => need file.
}

// In the trace, arrivals are described by resource requests >=0, departures as resource
// requests < 0.
func eventIsArrival(x float64) bool {
	return x >= 0
}
