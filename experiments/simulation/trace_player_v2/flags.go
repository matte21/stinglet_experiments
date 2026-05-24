package main

import (
	"flag"
	"log"
)

// The following vars are the simulator's CLI flags.
var (
	traceDBFile = flag.String("trace-db-file", "", "path to SQLite DB file with the trace (required)")
	cfgFile     = flag.String("config-file", "", "path to YAML config file (required)")
)

func parseFlagsOrDie() {
	flag.Parse()

	if *traceDBFile == "" {
		log.Fatal("--trace-db-file is required")
	}
	if *cfgFile == "" {
		log.Fatal("--config-file is required")
	}
}
