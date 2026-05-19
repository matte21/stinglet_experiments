package main

import (
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"strconv"
	"strings"

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

	printResults()
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
		vmID        int64
		priority    int64
		cpus        float64
		mem         float64
		totalCPUReq float64
		totalMemReq float64
	)
	for traceEvents.Next() {
		if err = traceEvents.Scan(&vmID, &priority, &cpus, &mem); err != nil {
			log.Fatalf("Failed to read row from trace of machine %d: %v", machineID, err)
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

func printResults() {
	for mid, stats := range machineIDToStats {
		fmt.Printf("mid: %d, cpus: %d, mem: %d, minNumHosts: %d\n",
			mid, len(stats.cpuReqsToCount), len(stats.memReqsToCount), stats.minNumHosts)
	}
}
