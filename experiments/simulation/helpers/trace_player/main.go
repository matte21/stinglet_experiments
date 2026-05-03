package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/coreos/go-systemd/v22/sdjournal"
)

var (
	traceDB              string
	resultsFilePrefix    string
	resultsDir           string
	cxlVMsPercentageName string
	machineID            int
)

type kubeletLogLineHandler struct {
	admitTimeFile   *os.File
	admitTimeRegexp *regexp.Regexp
}

func main() {
	parseFlagsOrDie()

	f := openAndInitAdmitTimesFileOrDie()
	defer func() {
		if err := f.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to close kubelet admit times result file %s: %v\n", f.Name(), err)
		}
	}()

	stop := make(chan time.Time)
	defer close(stop)
	startKubeletAdmitTimeCollectionOrDie(stop, &kubeletLogLineHandler{
		admitTimeFile:   f,
		admitTimeRegexp: regexp.MustCompile(`time="([^"]+)"`),
	})

	// play trace
}

// "Die", i.e., exit immediately, if the passed flags are invalid.
func parseFlagsOrDie() {
	flag.StringVar(&traceDB, "trace-db", "", "Path to the SQLite trace database file")
	flag.StringVar(&resultsFilePrefix, "results-file-prefix", "", "Prefix for result files names")
	flag.StringVar(&resultsDir, "results-dir", "", "Path to the directory where results are stored")
	flag.StringVar(&cxlVMsPercentageName, "cxl-vms-percentage-name", "", "Name of the CXL VMs percentage (e.g. pond, greensku, or nop)")
	flag.IntVar(&machineID, "machine-id", -1, "Azure machine type ID")

	flag.Parse()

	traceDB = strings.TrimSpace(traceDB)
	resultsFilePrefix = strings.TrimSpace(resultsFilePrefix)
	resultsDir = strings.TrimSpace(resultsDir)
	cxlVMsPercentageName = strings.ToLower(strings.TrimSpace(cxlVMsPercentageName))

	if traceDB == "" {
		fmt.Fprintln(os.Stderr, "Fatal: --trace-db is required and must be non-empty.")
		flag.Usage()
		os.Exit(1)
	}
	if resultsFilePrefix == "" {
		fmt.Fprintln(os.Stderr, "Fatal: --results-file-prefix is required and must be non-empty.")
		flag.Usage()
		os.Exit(1)
	}
	if resultsDir == "" {
		fmt.Fprintln(os.Stderr, "Fatal: --results-dir is required and must be non-empty.")
		flag.Usage()
		os.Exit(1)
	}
	if cxlVMsPercentageName == "" {
		fmt.Fprintln(os.Stderr, "Fatal: --cxl-vms-percentage-name is required, and must be one between greensku, pond, and nop.")
		flag.Usage()
		os.Exit(1)
	}
	if cxlVMsPercentageName != "pond" && cxlVMsPercentageName != "greensku" && cxlVMsPercentageName != "nop" {
		fmt.Fprintf(os.Stderr, "Fatal: --cxl-vms-percentage-name has value %s; must be one of: pond, greensku, nop.\n", cxlVMsPercentageName)
		flag.Usage()
		os.Exit(1)
	}
	if machineID == -1 {
		fmt.Fprintln(os.Stderr, "Fatal: --machine-id is required")
		flag.Usage()
		os.Exit(1)
	}
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

// example kubelet log line that this function matches and parses:
// Dec 09 14:43:19 vxf01 kubelet[1452415]: I1209 14:43:19.025926 1452415 topology_manager.go:232] "admit time" time="5.463692ms"
func (h *kubeletLogLineHandler) Write(l []byte) (int, error) {
	if bytes.Contains(l, []byte("admit time")) {
		admit_time := h.admitTimeRegexp.FindStringSubmatch(string(l))
		if len(admit_time) >= 2 {
			d, err := time.ParseDuration(admit_time[1])
			if err != nil {
				return 0, fmt.Errorf("failed to parse admit time duration %s: %w", admit_time[1], err)
			}
			usecs := d.Microseconds()
			if _, err := fmt.Fprintf(h.admitTimeFile, ",%d,,\n", usecs); err != nil {
				return 0, fmt.Errorf("failed to write admit time to %s: %w", h.admitTimeFile.Name(), err)
			}
		}
	}
	return len(l), nil
}
