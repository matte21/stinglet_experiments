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

const nopCXLPercentageColumn string = "nop"

var (
	machineIDToMinNumHosts       		map[int]uint32
	machineIDToCPURequestToCount 		map[int]map[float64]uint32
	machineIDToMemRequestToCount 		map[int]map[float64]uint32
	machineIDToBaselineRejectedCount 	map[int]uint32
	machineIDToBaselineHostsWastedCount map[int]uint32
)

func main() {
	parseFlagsOrDie()

	// Iterate over all machine IDs, and collect statistics for each machineID.
	for mid := range machineIDToMinNumHosts {
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
	machineIDToMinNumHosts 				= make(map[int]uint32, len(mIDs))
	machineIDToCPURequestToCount 		= make(map[int]map[float64]uint32, len(mIDs))
	machineIDToMemRequestToCount 		= make(map[int]map[float64]uint32, len(mIDs))
	machineIDToBaselineRejectedCount 	= make(map[int]uint32, len(mIDs))
	machineIDToBaselineHostsWastedCount = make(map[int]uint32, len(mIDs))
	for _, idStr := range mIDs {
		id, err := strconv.Atoi(strings.TrimSpace(idStr))
		if err != nil {
			fmt.Fprintln(os.Stderr, "Fatal: invalid --machine-ids argument."+
				" Format: --machine-ids <ID 1> or --machine-ids <ID 1>,<ID 2>, ...,<ID N>."+
				" The IDs must be integers.")
			flag.Usage()
			os.Exit(1)
		}
		machineIDToMinNumHosts[id] = 0
		machineIDToCPURequestToCount[id] = make(map[float64]uint32)
		machineIDToMemRequestToCount[id] = make(map[float64]uint32)
	}
}

func analyzeMachineIDOrDie(machineID int) {
	traceEvents, cleanupFunc, err := trace.Get(dbFilePath, machineID,
		nopCXLPercentageColumn)
	if err != nil {
		log.Fatalf("Failed to get trace: %v", err)
	}
	defer cleanupFunc()

	fmt.Printf("Analyzing trace for machine ID %d\n", machineID)

	cpuReqToCount := machineIDToCPURequestToCount[machineID]
	memReqToCount := machineIDToMemRequestToCount[machineID]
	var (
		vmID        int64
		priority    int64
		cpus        float64
		mem         float64
		totalCPUReq float64
		totalMemReq float64
		minNumHosts uint32
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

		if minNumHosts < uint32(math.Ceil(totalCPUReq)) {
			minNumHosts = uint32(math.Ceil(totalCPUReq))
		}
		if minNumHosts < uint32(math.Ceil(totalMemReq)) {
			minNumHosts = uint32(math.Ceil(totalMemReq))
		}

		cpuReqToCount[cpus]++
		memReqToCount[mem]++
	}

	machineIDToMinNumHosts[machineID] = minNumHosts
}

func printResults() {
	for mid := range machineIDToMinNumHosts {
		fmt.Printf("mid: %d, cpus: %d, mem: %d, minNumHosts: %d\n", mid, len(machineIDToCPURequestToCount[mid]),
			len(machineIDToMemRequestToCount[mid]), machineIDToMinNumHosts[[mid]])
	}
}
