package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"
	"time"

	k8sapi "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	k8sched "k8s.io/kubernetes/pkg/scheduler"
	k8schedfwk "k8s.io/kubernetes/pkg/scheduler/framework"
)

const (
	queryFmt = `SELECT vmId, priority, cores, memory, cxl_mem_percent_pond, cxl_mem_percent_greensku ` +
		`FROM "events-machine-id-%d" ORDER BY time`

	loggingPeriodEvents = 50000
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
	schedAlgo string
	agentName string
	hostType  string

	scheduler *k8sched.Scheduler

	// agent at index i runs on host at index i.
	agents          []hostAgent
	totHostMemBytes int64
	totHostCPUs     int64

	agentRejectReasonToCount map[string]uint64

	// These two bools tell whether the percentage of CXL memory that VMs/pod request is taken from
	// the Pond or GreenSKU version of the trace. They're mutually exclusive, and can both be false
	// (no VM/pod will use CXL).
	pondCXL     bool
	greenSKUCXL bool

	admittedVMIDToHost map[int64]int
	admittedVMIDToPod  map[int64]*k8sapi.Pod

	// Some statistics that are updated during the simulation.
	// todo: move arrivalsCount to the reader of a machineID since it's unique for a machine ID.
	arrivalsCount               uint64
	schedulerRejectsCount       uint64
	agentRejectsCount           uint64
	agentRejectsHostsWorthOfMem float64
	curHostsWorthOfMemUtil      float64
	avgHostsWorthOfMemUtil      float64
	avgHostsWorthOfMemUtilCount uint64
	admitTimeAvgUsec            float64
	admitTimeAvgCount           uint64
	schedTimeAvgUsec            float64
	schedTimeAvgCount           uint64

	schedRejectAvgMemRatio float64
	agentRejectAvgMemRatio float64
}

type work struct {
	machineID int

	// Event reader state.
	db       *sql.DB
	outChans []chan<- traceEvent

	// Event processor state.
	clusters []*cluster
	inChan   <-chan traceEvent
}

func worker(ctx context.Context, cancel context.CancelFunc, w work) {
	log.Printf("SIM: Worker for machine ID %d, reader=%t, processor=%t, STARTED\n",
		w.machineID, w.db != nil, len(w.outChans) == 0)
	defer func() {
		log.Printf("SIM: Worker for machine ID %d, reader=%t, processor=%t, TERMINATED\n",
			w.machineID, w.db != nil, len(w.outChans) == 0)

		if len(w.outChans) == 0 {
			printStats(w)
		}
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

func printStats(w work) {
	for _, c := range w.clusters {
		log.Printf("SIM: STATS: mid=%d, config=%s-%s-%s, arrivals=%d, sched_rejects=%d, agent_rejects=%d, avg_mem_util_hosts=%f, agent_rejects_cum_mem_util_hosts=%f, sched_avg_us=%f, admit_avg_us=%f\n",
			w.machineID,
			c.schedAlgo,
			c.agentName,
			c.hostType,
			c.arrivalsCount,
			c.schedulerRejectsCount,
			c.agentRejectsCount,
			c.avgHostsWorthOfMemUtil,
			c.agentRejectsHostsWorthOfMem,
			c.schedTimeAvgUsec,
			c.admitTimeAvgUsec,
		)
	}
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
			if err := process(ctx, event, w.clusters); err != nil {
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
	query := fmt.Sprintf(queryFmt, w.machineID)
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

	var eventsRead uint64

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
			if err := process(ctx, e, w.clusters); err != nil {
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

		eventsRead++
		if eventsRead%loggingPeriodEvents == 0 {
			log.Printf("SIM: mid=%d dispatched %d events\n", w.machineID, eventsRead)
		}
	}

	if err := trace.Err(); err != nil {
		log.Printf("SIM: ERROR: reader of machine ID %d failed to prepare next row: %v\n",
			w.machineID, err)
	}
}

func process(ctx context.Context, e traceEvent, clusters []*cluster) error {
	for _, c := range clusters {
		if e.isArrival {
			c.arrivalsCount++

			p := newPodFromEvent(e, c.totHostMemBytes, c.totHostCPUs)

			o, err := schedule(ctx, c, p)
			if err != nil {
				var fitErr *k8schedfwk.FitError
				if errors.As(err, &fitErr) {
					c.schedulerRejectsCount++
					c.schedRejectAvgMemRatio += (e.vmMem - c.schedRejectAvgMemRatio) / float64(c.schedulerRejectsCount)
					continue
				}
				return fmt.Errorf("scheduling of pod %s failed with error other than fit error: %w", p.Name, err)
			}
			p.Spec.NodeName = o.SuggestedHost

			// TODO: this parsing/unparsing sucks, switch to storing hosts on a map keyed by name to
			// avoid it. Hosts have name "host-<i>" where i is their index in the hosts array.
			_, agentI, _ := strings.Cut(o.SuggestedHost, "-")
			i, err := strconv.ParseInt(agentI, 10, 64)
			if err != nil {
				return fmt.Errorf("got malformed host name %s from scheduler: %v\n", o.SuggestedHost, err)
			}
			if rejectReason, err := admit(c, i, p); err != nil {
				c.agentRejectsCount++
				c.agentRejectsHostsWorthOfMem += e.vmMem

				c.avgHostsWorthOfMemUtilCount++
				c.avgHostsWorthOfMemUtil += (c.curHostsWorthOfMemUtil - c.avgHostsWorthOfMemUtil) / float64(c.avgHostsWorthOfMemUtilCount)

				c.agentRejectReasonToCount[rejectReason]++

				c.agentRejectAvgMemRatio += (e.vmMem - c.agentRejectAvgMemRatio) / float64(c.agentRejectsCount)

				continue
			}

			if err := c.scheduler.Cache.AddPod(klog.Background(), p); err != nil {
				return fmt.Errorf("scheduler failed to add pod %s to cache: %v", p.Name, err)
			}

			c.admittedVMIDToHost[e.vmID] = int(i)
			c.admittedVMIDToPod[e.vmID] = p

			c.curHostsWorthOfMemUtil += e.vmMem
			c.avgHostsWorthOfMemUtilCount++
			c.avgHostsWorthOfMemUtil += (c.curHostsWorthOfMemUtil - c.avgHostsWorthOfMemUtil) / float64(c.avgHostsWorthOfMemUtilCount)
		} else {
			if h, ok := c.admittedVMIDToHost[e.vmID]; ok {
				pod := c.admittedVMIDToPod[e.vmID]
				if err := c.agents[h].HandleDeparture(pod.Name); err != nil {
					return fmt.Errorf("agent failed to remove %s: %w", pod.Name, err)
				}
				if err := c.scheduler.Cache.RemovePod(klog.Background(), pod); err != nil {
					return fmt.Errorf("scheduler failed to remove %s: %w", pod.Name, err)
				}
				delete(c.admittedVMIDToHost, e.vmID)
				delete(c.admittedVMIDToPod, e.vmID)
				// e.vmMem is negative for departure events, so we can just add it.
				c.curHostsWorthOfMemUtil += e.vmMem
			}
		}
	}

	return nil
}

func schedule(ctx context.Context, c *cluster, p *v1.Pod) (k8sched.ScheduleResult, error) {
	t0 := time.Now()
	r, err := c.scheduler.SimSchedulePod(ctx, p)
	d := time.Since(t0).Microseconds()

	c.schedTimeAvgCount++
	c.schedTimeAvgUsec += (float64(d) - c.schedTimeAvgUsec) / float64(c.schedTimeAvgCount)

	return r, err
}

func admit(c *cluster, i int64, p *v1.Pod) (string, error) {
	t0 := time.Now()
	rejectReason, err := c.agents[i].Admit(p)
	d := time.Since(t0).Microseconds()

	c.admitTimeAvgCount++
	c.admitTimeAvgUsec += (float64(d) - c.admitTimeAvgUsec) / float64(c.admitTimeAvgCount)

	return rejectReason, err
}

func newPodFromEvent(e traceEvent, hostBytes, hostCPUs int64) *v1.Pod {
	podBytes := int64(float64(hostBytes) * e.vmMem)
	podCPUsFlt := float64(hostCPUs) * e.vmCPUs

	// Guaranteed Pods must request an integer number of cores to get exclusive CPUs by the CPU
	// manager as per K8s API specification. But the raw trace data doesn't respect that
	// constraint. We enforce it by rounding up.
	var podCPUs int64
	if e.vmPriority == 0 {
		podCPUs = int64(math.Ceil(podCPUsFlt))
	} else {
		podCPUs = int64(podCPUsFlt)
	}

	return newPod(fmt.Sprintf("pod-%d", e.vmID), podBytes, podCPUs, e.vmPriority == 0)
}

// Note: CPUs unit is whole cores, not millicores.
func newPod(name string, bytes, cpus int64, guaranteed bool) *v1.Pod {
	p := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			UID:  types.UID(name),
		},
		Spec: v1.PodSpec{
			SchedulerName: miniDefaultK8sScheduler,
			Containers: []v1.Container{
				{
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceCPU:    *resource.NewMilliQuantity(cpus*1000, resource.DecimalSI),
							v1.ResourceMemory: *resource.NewQuantity(bytes, resource.BinarySI),
						},
					},
				},
			},
		},
	}

	if guaranteed {
		p.Spec.Containers[0].Resources.Limits = p.Spec.Containers[0].Resources.Requests.DeepCopy()
	}

	return p
}

func newHost(name string, bytes, cpus int64) *v1.Node {
	return &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			UID:  types.UID(name),
		},
		Status: v1.NodeStatus{
			Capacity: v1.ResourceList{
				v1.ResourceCPU:    *resource.NewMilliQuantity(cpus*1000, resource.DecimalSI),
				v1.ResourceMemory: *resource.NewQuantity(bytes, resource.BinarySI),
			},
			Allocatable: v1.ResourceList{
				v1.ResourceCPU:    *resource.NewMilliQuantity(cpus*1000, resource.DecimalSI),
				v1.ResourceMemory: *resource.NewQuantity(bytes, resource.BinarySI),
				v1.ResourcePods:   *resource.NewQuantity(110, resource.DecimalSI),
			},
		},
	}
}
