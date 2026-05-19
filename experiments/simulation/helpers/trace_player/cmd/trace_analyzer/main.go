package main

import (
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"trace_player/internal/trace"
)

// These variables are all the CLI flags.
var (
	dbFilePath     string
	machineIDsList string
)

const (
	nopCXLPercentageColumn string = "nop"

	vlabNNUMAGiB float64 = 252
	vlabTotGiB   float64 = 520

	s4NNUMAGiB float64 = 126
	s4TotGiB   float64 = 568

	// To (guess)estimate the rejections under the baseline, we assume any VM requiring more than
	// one nNUMA's worth of memory is rejected, due to kubelet's rule that apps can share NUMA nodes
	// for memory only if they're identical. The following consts hold the minimum memory request
	// (as a fraction of the host's total) that causes a VM to count as rejected in our estimate.
	vlabRejectionMemReq float64 = vlabNNUMAGiB / vlabTotGiB
	s4RejectionMemReq   float64 = s4NNUMAGiB / s4TotGiB

	// How many trace events between consecutive log statements that inform users about how far
	// in playing the trace we are. I've found 300000 to be a good trade-off between responsiveness
	// and conciseness.
	traceEventsLoggingPeriod uint32 = 300000
)

var machineIDToStats map[int]*machineIDStats

type machineIDStats struct {
	totalVMs uint32

	minNumHosts uint32

	cpuReqsToCount map[float64]uint32
	memReqsToCount map[float64]uint32

	vlabBaselineRejectionsCount     uint32
	vlabBaselineHostsWastedCapacity float64

	s4BaselineRejectionsCount     uint32
	s4BaselineHostsWastedCapacity float64
}

func main() {
	parseFlagsOrDie()

	// Iterate over all machine IDs, and collect statistics for each machineID.
	for mid := range machineIDToStats {
		analyzeMachineIDOrDie(mid)
	}

	printStats()
}

func parseFlagsOrDie() {
	flag.StringVar(&dbFilePath, "db-file-path", "",
		"Path to the SQLite trace database file")
	flag.StringVar(&machineIDsList, "machine-ids", "",
		"Comma-separated list of the IDs of the azure trace machine types that we want to analyze")

	flag.Parse()

	dbFilePath = strings.TrimSpace(dbFilePath)
	if dbFilePath == "" {
		fmt.Fprintln(os.Stderr, "Fatal: --db-file-path is required and must be non-empty.")
		flag.Usage()
		os.Exit(1)
	}

	// Parse the machine IDs list.
	mIDs := strings.Split(strings.TrimSpace(machineIDsList), ",")
	if len(mIDs) == 0 {
		fmt.Fprintln(os.Stderr, "Fatal: need at least one machine ID, none provided."+
			" Format: --machine-ids <ID 1> or --machine-ids <ID 1>,<ID 2>, ...,<ID N>."+
			" The IDs must be integers")
		flag.Usage()
		os.Exit(1)
	}
	machineIDToStats = make(map[int]*machineIDStats, len(mIDs))
	for _, idStr := range mIDs {
		id, err := strconv.Atoi(strings.TrimSpace(idStr))
		if err != nil {
			fmt.Fprintln(os.Stderr, "Fatal: invalid --machine-ids argument."+
				" Format: --machine-ids <ID 1> or --machine-ids <ID 1>,<ID 2>, ...,<ID N>."+
				" The IDs must be integers.")
			flag.Usage()
			os.Exit(1)
		}
		machineIDToStats[id] = &machineIDStats{
			cpuReqsToCount: make(map[float64]uint32),
			memReqsToCount: make(map[float64]uint32),
		}
	}
}

func analyzeMachineIDOrDie(machineID int) {
	traceEvents, cleanupFunc, err := trace.Get(dbFilePath, machineID, nopCXLPercentageColumn)
	if err != nil {
		log.Fatalf("Failed to get trace: %v", err)
	}
	defer cleanupFunc()

	fmt.Printf("Analyzing trace for machine ID %d\n", machineID)

	stats := machineIDToStats[machineID]
	var (
		vmID                 int64
		priority             int64
		cpus                 float64
		mem                  float64
		totalCPUReq          float64
		totalMemReq          float64
		traceEventsProcessed uint32
	)
	for traceEvents.Next() {
		if err = traceEvents.Scan(&vmID, &priority, &cpus, &mem); err != nil {
			log.Fatalf("Failed to read row from trace of machine %d: %v", machineID, err)
		}

		traceEventsProcessed++
		if traceEventsProcessed%traceEventsLoggingPeriod == 0 {
			fmt.Printf("Processed %d trace events\n", traceEventsProcessed)
		}

		// Increase total current amount of CPU and memory requests, or decrease it if the event is
		// a departure. Departures are encoded as rows with negative cpus/mem, so we can just
		// add them to decrement the total.
		totalCPUReq += cpus
		totalMemReq += mem

		// If the event is a departure, we don't care about it. Departures are encoded as rows where
		// the cpus and memory requests are negative (yikes).
		if cpus < 0 {
			continue
		}

		if cr := uint32(math.Ceil(totalCPUReq)); stats.minNumHosts < cr {
			stats.minNumHosts = cr
		}
		if mr := uint32(math.Ceil(totalMemReq)); stats.minNumHosts < mr {
			stats.minNumHosts = mr
		}

		stats.totalVMs++
		stats.cpuReqsToCount[cpus]++
		stats.memReqsToCount[mem]++

		if mem >= vlabRejectionMemReq {
			stats.vlabBaselineRejectionsCount++
			stats.vlabBaselineHostsWastedCapacity += mem
		}

		if mem >= s4RejectionMemReq {
			stats.s4BaselineRejectionsCount++
			stats.s4BaselineHostsWastedCapacity += mem
		}
	}
}

func printStats() {
	// Create the directory that will hold all files with the stats.
	resDir := createStatsDirOrDie()

	for mid, stats := range machineIDToStats {
		printMachineIDStats(mid, stats, resDir)
	}
}

func createStatsDirOrDie() string {
	now := time.Now().Format("2006-01-02-15-04-05")
	resDir := "trace_analysis/" + now
	if err := os.MkdirAll(resDir, 0755); err != nil {
		log.Fatalf("Failed to create results directory %s: %v", resDir, err)
	}
	return resDir
}

func printMachineIDStats(mid int, stats *machineIDStats, resDir string) {
	// Create and open file to which we'll write the stats.
	statsFilePath := fmt.Sprintf("%s/machine-id-%d.csv", resDir, mid)
	f, err := os.Create(statsFilePath)
	if err != nil {
		fmt.Printf("Failed to create stats file %s for machine ID %d: %v\n",
			statsFilePath, mid, err)
		// do not panic or exit: there are other machine IDs whose stats we might be able to
		// print.
		return
	}
	defer func() {
		if err := f.Close(); err != nil {
			fmt.Printf("Failed to close stats file %s for machine ID %d: %v\n",
				statsFilePath, mid, err)
		}
	}()

	// Print summary stats.
	fmt.Fprintf(f, "num VMs,%d,\n"+
		"min num hosts,%d,\n"+
		"s4 baseline num rejected VMs,%d,\n"+
		"s4 baseline wasted hosts,%.2f,\n"+
		"s4 baseline wasted hosts percentage,%.2f,\n"+
		"vlab baseline num rejected VMs,%d,\n"+
		"vlab baseline wasted hosts,%.2f,\n"+
		"vlab baseline wasted hosts percentage,%.2f,\n"+
		"num distinct cpu requests,%d,\n"+
		"num distinct mem requests,%d,\n\n",
		stats.totalVMs,
		stats.minNumHosts,
		stats.s4BaselineRejectionsCount,
		stats.s4BaselineHostsWastedCapacity,
		100*stats.s4BaselineHostsWastedCapacity/float64(stats.minNumHosts),
		stats.vlabBaselineRejectionsCount,
		stats.vlabBaselineHostsWastedCapacity,
		100*stats.vlabBaselineHostsWastedCapacity/float64(stats.minNumHosts),
		len(stats.cpuReqsToCount),
		len(stats.memReqsToCount))

	cpuReqsCSVLines := formatReqsCountsForCSVFile(stats.cpuReqsToCount)
	memReqsCSVLines := formatReqsCountsForCSVFile(stats.memReqsToCount)

	cpuLinesHeading := "CPU requests histogram,"
	memLinesHeading := "Memory requests histogram,"
	fmt.Fprintf(f, "%s\n%s\n\n%s\n%s\n",
		cpuLinesHeading, cpuReqsCSVLines, memLinesHeading, memReqsCSVLines)
}

func formatReqsCountsForCSVFile(reqsToCount map[float64]uint32) string {
	sortedReqs := make([]float64, 0, len(reqsToCount))
	for r := range reqsToCount {
		sortedReqs = append(sortedReqs, r)
	}
	slices.Sort(sortedReqs)

	// these "histograms" are strings formatted as CSV file rows.
	reqsHistogramHeadings := strings.Builder{}
	reqsHistogramVals := strings.Builder{}

	reqsHistogramHeadings.WriteString("request,")
	reqsHistogramVals.WriteString("VM count,")

	for _, r := range sortedReqs {
		fmt.Fprintf(&reqsHistogramHeadings, "%.2f,", r)
		fmt.Fprintf(&reqsHistogramVals, "%d,", reqsToCount[r])
	}

	// Concatenate the heading and values lines.
	fmt.Fprintf(&reqsHistogramHeadings, "\n%s", reqsHistogramVals.String())

	return reqsHistogramHeadings.String()
}
