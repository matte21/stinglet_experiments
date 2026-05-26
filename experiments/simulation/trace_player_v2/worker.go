package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	k8sapi "k8s.io/api/core/v1"
	k8sched "k8s.io/kubernetes/pkg/scheduler"
)

type traceEvent struct {
	isArrival               bool
	vmID                    int64
	vmPriority              int64
	vmCPUs                  float64
	vmMem                   float64
	vmCXLMemPercentPond     sql.NullInt32
	vmCXLMemPercentGreenSKU sql.NullInt32
}

type cluster struct {
	scheduler *k8sched.Scheduler

	// agent at index i runs on host at index i.
	agents []hostAgent
	hosts  []*k8sapi.Node

	// These two bools tell whether the percentage of CXL memory that VMs/pod request is taken from
	// the Pond or GreenSKU version of the trace. They're mutually exclusive, and can both be false
	// (no VM/pod will use CXL).
	pondCXL     bool
	greenSKUCXL bool
}

type work struct {
	machineID int

	// Event reader state.
	db       *sql.DB
	outChans []chan<- traceEvent

	// Event processor state.
	clusters []cluster
	inChan   <-chan traceEvent
}

func worker(ctx context.Context, cancel context.CancelFunc, w work) {
	log.Printf("SIM: Worker for machine ID %d, reader=%t, processor=%t, STARTED\n",
		w.machineID, w.db != nil, len(w.outChans) == 0)
	defer func() {
		log.Printf("SIM: Worker for machine ID %d, reader=%t, processor=%t, TERMINATED\n",
			w.machineID, w.db != nil, len(w.outChans) == 0)
	}()

	if w.db == nil {
		// If we're here, this worker is a processor of trace events, but not a reader. The events
		// will be read by the trace DB and sent to it by another worker.
		processEvents(ctx, cancel, w)
		return
	}

	// If we're here, this worker is a reader of trace events: it must read them from the sqlite DB.
	// It may either process them directly or send them to processors, depending on how this program
	// was invoked.
	readAndSendOrProcessEvents(ctx, w)
}

func processEvents(ctx context.Context, cancel context.CancelFunc, w work) {
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-w.inChan:
			if !ok {
				// channel closed, either we've played all of the trace or the reader of the
				// trace (that sends us the trace events) encountered an error and quit early.
				return
			}
			if err := process(event); err != nil {
				log.Printf("SIM: ERROR: processor of machine ID %d failed to process event: %v\n",
					w.machineID, err)
				cancel()
				return
			}
		}
	}
}

func readAndSendOrProcessEvents(ctx context.Context, w work) {
	defer func() {
		// When done reading the trace (either because we successfully read it all, or because an
		// error was encountered), signal to the event processors that we're finished, so that they
		// know they have to stop.
		for _, ch := range w.outChans {
			close(ch)
		}
	}()

	// Query the DB for the trace/machineID this worker is responsible for.
	// TODO: the query format should be a const.
	query := fmt.Sprintf(`SELECT vmId, priority, cores, memory, cxl_mem_percent_pond, cxl_mem_percent_greensku `+
		`FROM "events-machine-id-%d" ORDER BY time`, w.machineID)
	trace, err := w.db.Query(query)
	if err != nil {
		log.Printf("SIM: ERROR: reader of machine ID %d failed to run query \"%s\": %v\n",
			w.machineID, query, err)
		return
	}
	defer func() {
		if err := trace.Close(); err != nil {
			log.Printf("SIM: WARNING: reader of machine ID %d failed to close DB connection: %v\n",
				w.machineID, err)
		}
	}()

	// Now read the trace events from the DB, and either process them directly or send them
	// directly.
	e := traceEvent{}
	for trace.Next() {
		if err := trace.Scan(
			&e.vmID,
			&e.vmPriority,
			&e.vmCPUs,
			&e.vmMem,
			&e.vmCXLMemPercentPond,
			&e.vmCXLMemPercentGreenSKU,
		); err != nil {
			log.Printf("SIM: ERROR: reader of machine ID %d failed to scan row: %v\n", w.machineID, err)
			return
		}

		// In the DB, departures are modeled implicitly by setting negative CPUs or memory for the
		// VM.
		e.isArrival = e.vmCPUs > 0

		if len(w.outChans) == 0 {
			// If we're here, this worker is both a reader and a processor of events, so we
			// process them.
			if err := process(e); err != nil {
				log.Printf("SIM: ERROR: processor of machine ID %d failed to process event: %v\n",
					w.machineID, err)
				return
			}
		} else {
			// If we're here, this worker is a reader but not a processor of events, so we send
			// the event to the processors.
			for _, ch := range w.outChans {
				select {
				case <-ctx.Done():
					// If we're here, one of the processors encountered an error, so we exit early
					return
				case ch <- e:
				}
			}
		}
	}

	if err := trace.Err(); err != nil {
		log.Printf("SIM: ERROR: reader of machine ID %d failed to prepare next row: %v\n",
			w.machineID, err)
	}
}

func process(e traceEvent) error {
	// TODO: implement me!
	return nil
}
