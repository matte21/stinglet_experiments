package main

import (
	"bytes"
	"database/sql"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"text/template"
	"time"

	"github.com/coreos/go-systemd/v22/sdjournal"
	k8sapi "k8s.io/api/core/v1"
	k8sresources "k8s.io/apimachinery/pkg/api/resource"
)

// CLI flags.
var (
	resultsDir            string
	resultsFilePrefix     string
	traceDB               string
	traceMachineID        int
	cxlPercentageDBColumn string
)

// TODO: check that our logic on log filtering makes sense: we might see the admit time even on rejections!

// The following variables track some statistics we care about, either for the paper results, or
// as sanity checks.
// All names are self-explanatory, except for ignoredPodsCount. It counts the pods that haven't
// been scheduled to our reference host because it didn't have enough free memory and CPUs.
var (
	podsCount                    uint32
	ignoredPodsCount             uint32
	rejectedPodsCount            uint32
	admittedPodsCount            uint32
	podsThatUseCXLCount          uint32
	admittedPodsThatUseCXLCount  uint32
	admitTimeSamplesCount        atomic.Uint32
	admitTimeSamplesWrittenCount uint32
)

// totals are used to scale the VM/pods resource requests (which are relative to the host's capacity).
// frees are used to check whether a VM/pod fits on the host where stinglet/kubelet is running.
var (
	// TODO: init total values.
	hostTotalBytes int64
	hostFreeBytes  int64

	hostTotalMilliCores int64
	hostFreeMilliCores  int64
)

var (
	//go:embed pod_manifest.yaml.tmpl
	podManifestTemplateStr string
	podManifestTemplate    *template.Template
	// Map to store values of per-pod fields that will be used to render the manifest out of the
	// template.
	podFields map[string]any
)

const (
	staticPodsDir = "/etc/kubernetes/manifests"
	podsHealthAPI = "http://localhost:10255/pods"

	unexpectedAdmissionErr = "UnexpectedAdmissionError"
)

var kubeletClient *http.Client

type kubeletLogLineHandler struct {
	admitTimeSamplesFile *os.File
	admitTimeRegexp      *regexp.Regexp
}

func main() {
	parseFlagsOrDie()

	podManifestTemplate = template.Must(template.New("pod-manifest").Parse(podManifestTemplateStr))
	podFields = make(map[string]any, 6)

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

	kubeletClient = &http.Client{
		Timeout: 5 * time.Second,
	}

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

// todo: consider error handling within the trace.
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
			if err := handleArrival(priority, cores, memory, cxlMemPercent); err != nil {
				return fmt.Errorf("failed to handle arrival: %w", err)
			}
		} else {
			// process departure.
		}
	}

	return nil
}

func handleArrival(priority int, normalizedCores, normalizedMemory float64, cxlMemPercent sql.NullInt32) error {
	podsCount++

	requestedBytes := int64(float64(hostTotalBytes) * normalizedMemory)
	requestedCXLBytes := int64(float64(requestedBytes) * (float64(cxlMemPercent.Int32) / 100))
	requestedBytes -= requestedCXLBytes
	requestedMilliCores := int64(float64(hostTotalMilliCores) * normalizedCores)

	isGuaranteed := podIsGuaranteed(priority)

	// Guaranteed Pods must request an integer number of cores to get exclusive CPUs by the CPU
	// manager as per K8s API specification. But the raw trace data doesn't respect that
	// constraint. We enforce it by rounding up.
	if isGuaranteed && requestedMilliCores%1000 != 0 {
		requestedMilliCores += 1000 - (requestedMilliCores % 1000)
	}

	if cxlMemPercent.Valid && cxlMemPercent.Int32 > 0 {
		podsThatUseCXLCount++
	}

	// If the pod doesn't fit on our reference host we ignore it - it's as if the cluster scheduler
	// scheduled it to a different host.
	if requestedBytes > hostFreeBytes || requestedMilliCores > hostFreeMilliCores {
		ignoredPodsCount++
		return nil
	}

	// If we got here, the host has enough resources to fit the pod, in principle (but intra-host
	// topology constraints might still cause the pod rejection).

	if err := createPod(podsCount, requestedCXLBytes, requestedBytes, requestedMilliCores, isGuaranteed); err != nil {
		return fmt.Errorf("failed to create Pod %d: %w", podsCount, err)
	}

	// Poll kubelet API to know whether the Pod was admitted or rejected.
	rejected, err := podRejected(podsCount)
	if err != nil {
		return fmt.Errorf("failed to read status of created pod %d: %w", podsCount, err)
	}
	if rejected {
		rejectedPodsCount++
		return nil
	}

	// todo handle admission

	admittedPodsCount++
	// If accepted.
	// Add to active set.
	// read kubelet files -- need connection.
	// update resource availability.
	// sample memory utilization => need file.
}

// In the trace, arrivals are described by resource requests >=0, departures as resource
// requests < 0.
func eventIsArrival(resourceRequest float64) bool {
	return resourceRequest >= 0
}

func podIsGuaranteed(priority int) bool {
	return priority == 0
}

func createPod(id uint32, cxlBytes, bytes, milliCores int64, isGuaranteed bool) error {
	podFields["ID"] = id
	podFields["CXLMemRequest"] = k8sresources.NewQuantity(cxlBytes, k8sresources.BinarySI).String()
	podFields["MemoryRequest"] = k8sresources.NewQuantity(bytes, k8sresources.BinarySI).String()
	podFields["MemoryLimit"] = podFields["MemoryRequest"]
	podFields["MilliCoresRequest"] = milliCores
	if isGuaranteed {
		podFields["MilliCoresLimit"] = milliCores
	} else {
		// We want the pod to be burstable => per K8s API, its cpu (or memory) limits must be
		// higher than the requests.
		podFields["MilliCoresLimit"] = milliCores + 1
	}

	podFileName := podFileName(id)
	f, err := os.Create(podFileName)
	if err != nil {
		return fmt.Errorf("failed to create Pod manifest file %s: %w", podFileName, err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to close pod manifest file %s: %s", podFileName, err)
		}
	}()

	if err := podManifestTemplate.Execute(f, podFields); err != nil {
		return fmt.Errorf("failed to write Pod manifest file %s: %w", podFileName, err)
	}

	return nil
}

// podRejected spins until the kubelet API tells us whether the pod has been accepted or rejected.
func podRejected(podID uint32) (bool, error) {
	var i uint64
	podName := "p-" + strconv.Itoa(int(podID))
	for {
		resp, err := kubeletClient.Get(podsHealthAPI)
		if err != nil {
			return false, fmt.Errorf("failed to GET pods from kubelet API %s: %w", podsHealthAPI, err)
		}

		var allPods k8sapi.PodList
		err = json.NewDecoder(resp.Body).Decode(&allPods)
		if closeErr := resp.Body.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "Failed to close HTTP response from kubelet API %s: %s\n",
				podsHealthAPI, closeErr)
		}
		if err != nil {
			return false, fmt.Errorf("failed to decode pods list from kubelet API %s into Go struct: %w",
				podsHealthAPI, err)
		}

		for _, p := range allPods.Items {
			if p.Name != podName {
				continue
			}
			if p.Status.Phase == k8sapi.PodRunning {
				return false, nil
			}
			if p.Status.Phase == k8sapi.PodFailed {
				// Memove the pod manifest from kubelet's static pod dir
				podFileName := podFileName(podID)
				if err := os.Remove(podFileName); err != nil {
					fmt.Fprintf(os.Stderr, "Failed to delete file %s for pod %s: %s\n", podFileName, podName, err)
				}

				// Check whether the pod has been rejected or instead failed for unexpected reasons.
				reason := p.Status.Reason
				if reason == "" && len(p.Status.ContainerStatuses) > 0 && p.Status.ContainerStatuses[0].State.Terminated != nil {
					reason = p.Status.ContainerStatuses[0].State.Terminated.Reason
				}
				if reason == unexpectedAdmissionErr {
					// These rejections are ok: they mean the kubelet resource managers couldn't place the pod.
					return true, nil
				}
				return false, fmt.Errorf("pod %s failed for reason %s, it should never happen; only expected reason is %s",
					podName, reason, unexpectedAdmissionErr)
			}
		}

		time.Sleep(100 * time.Millisecond)
		i++
		if i%30 == 0 {
			fmt.Printf("Waiting for pod %s to be either running or failed...\n", podName)
		}
	}
}

func podFileName(podID uint32) string {
	return filepath.Join(staticPodsDir, strconv.Itoa(int(podID))+".yaml")
}
