package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	_ "trace_player/internal/trace"
)

// These variables are all the CLI flags.
var (
	dbFilePath     string
	machineIDsList string
)

// const cxlPercentageColumn string = "nop"

var (
	machineIDToMinNumHosts       map[int]uint32
	machineIDToCPURequestToCount map[int]map[float64]uint32
	machineIDToMemRequestToCount map[int]map[float64]uint32
)

func main() {
	parseFlagsOrDie()
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
	machineIDToMinNumHosts = make(map[int]uint32, len(mIDs))
	machineIDToCPURequestToCount = make(map[int]map[float64]uint32, len(mIDs))
	machineIDToMemRequestToCount = make(map[int]map[float64]uint32, len(mIDs))
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
