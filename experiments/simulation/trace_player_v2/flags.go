package main

import (
	"flag"
	"log"
	"runtime"
)

// The following vars are the simulator's CLI flags.
var (
	traceDBFile = flag.String("trace-db-file", "",
		"path to SQLite DB file with the trace (required)")
	cfgFile    = flag.String("config-file", "", "path to YAML config file (required)")
	numWorkers = flag.Int("num-workers", runtime.GOMAXPROCS(0),
		"number of worker goroutines (default: GOMAXPROCS)")
)

func parseFlagsOrDie() {
	flag.Parse()

	if *traceDBFile == "" {
		log.Fatal("SIM: --trace-db-file is required")
	}
	if *cfgFile == "" {
		log.Fatal("SIM: --config-file is required")
	}
	if *numWorkers <= 0 {
		log.Fatal("SIM: --num-workers must be > 0")
	}
}
