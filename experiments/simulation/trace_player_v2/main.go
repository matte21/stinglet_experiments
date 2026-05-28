package main

import (
	"context"
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/go-logr/logr"
	cadvisorapi "github.com/google/cadvisor/info/v1"
	v1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/informers"
	clientsetfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/events"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler"
	schedcfg "k8s.io/kubernetes/pkg/scheduler/apis/config"
	"k8s.io/kubernetes/pkg/scheduler/profile"
	_ "modernc.org/sqlite"
)

var Version = "dev"

type hostAgent interface {
	Admit(*v1.Pod) (string, error)
	HandleDeparture(podName string) error
}

// Note: currently we assume that SMT is enabled with two threads per core.
const (
	threadsPerCore   = 2
	reservedCPUs     = threadsPerCore
	reservedMemBytes = 1 << 12
	// The following values are taken from S4.
	intraNUMADistance   = 10
	intraSocketDistance = 12
	outOfSocketDistance = 21

	miniDefaultK8sScheduler = "mini-default-k8s-sched"
)

func main() {
	klog.SetOutput(io.Discard)
	klog.SetLogger(logr.Discard())

	log.Printf("SIM: Running %s\n", Version)

	parseFlagsOrDie()

	cfg := parseCfgOrDie(*cfgFile)

	db, closeConnFunc := connectToTraceDBOrDie(*traceDBFile, len(cfg.Clusters))
	defer closeConnFunc()

	// TODO: a single ctx/cancel fot all workers is too coarse-grained, we need to switch to
	// per-machineID contexts.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var leftoversMux sync.Mutex
	workItems, workLeftovers := prepareWorkOrDie(ctx, db, cfg, *numWorkers)

	// Now, actually start the workers.
	var wg sync.WaitGroup
	wg.Add(*numWorkers)
	for i := 0; i < *numWorkers; i++ {
		go func(j int) {
			defer wg.Done()

			for {
				worker(ctx, cancel, workItems[j])

				// If there are work leftovers, complete one.
				leftoversMux.Lock()
				if len(workLeftovers) > 0 {
					for j = range workLeftovers {
						break
					}
					delete(workLeftovers, j)
					leftoversMux.Unlock()
					continue
				}
				leftoversMux.Unlock()
				return
			}
		}(i)
	}

	wg.Wait()

	printResultsOrDie(workItems)
}

// TODO: make this perversely inefficient function more efficient.
func printResultsOrDie(workItems []work) {
	for _, w := range workItems {
		for _, c := range w.clusters {
			fileName := fmt.Sprintf("%s/%s-%d-%s-%s.csv", *outputDir, *outputFilePrefix, w.machineID, c.schedAlgo, c.hostType)

			f, err := os.OpenFile(fileName, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
			if err != nil {
				log.Fatalf("failed to open results file %s: %v\n", fileName, err)
			}

			func() {
				defer func() {
					if err := f.Close(); err != nil {
						log.Printf("failed to close results file %s: %v\n", fileName, err)
					}
				}()

				info, err := f.Stat()
				if err != nil {
					log.Fatalf("failed to stat results file %s: %v\n", fileName, err)
				}

				wtr := csv.NewWriter(f)

				if info.Size() == 0 {
					if err := wtr.Write([]string{
						"agent",
						"arrivalsCount",
						"schedulerRejectsCount",
						"agentRejectsCount",
						"agentRejectsHostsWorthOfMem",
						"avgHostsWorthOfMemUtil",
						"avgHostsWorthOfCPUUtil",
						"admitTimeAvgUsec",
						"schedTimeAvgUsec",
						"schedRejectAvgMemRatio",
						"agentRejectAvgMemRatio",
					}); err != nil {
						log.Fatalf("failed to write summary header to %s: %v\n", fileName, err)
					}
				}

				if err := wtr.Write([]string{
					c.agentName,
					strconv.FormatUint(c.arrivalsCount, 10),
					strconv.FormatUint(c.schedulerRejectsCount, 10),
					strconv.FormatUint(c.agentRejectsCount, 10),
					strconv.FormatFloat(c.agentRejectsHostsWorthOfMem, 'f', -1, 64),
					strconv.FormatFloat(c.avgHostsWorthOfMemUtil, 'f', -1, 64),
					strconv.FormatFloat(c.avgHostsWorthOfCPUUtil, 'f', -1, 64),
					strconv.FormatFloat(c.admitTimeAvgUsec, 'f', -1, 64),
					strconv.FormatFloat(c.schedTimeAvgUsec, 'f', -1, 64),
					strconv.FormatFloat(c.schedRejectAvgMemRatio, 'f', -1, 64),
					strconv.FormatFloat(c.agentRejectAvgMemRatio, 'f', -1, 64),
				}); err != nil {
					log.Fatalf("failed to write summary row to %s: %v\n", fileName, err)
				}

				wtr.Flush()
				if err := wtr.Error(); err != nil {
					log.Fatalf("failed to flush summary csv writer for %s: %v\n", fileName, err)
				}
			}()
		}
	}

	for _, w := range workItems {
		for _, c := range w.clusters {
			fileName := fmt.Sprintf("%s/%s-%d-%s-%s.csv", *outputDir, *outputFilePrefix, w.machineID, c.schedAlgo, c.hostType)

			f, err := os.OpenFile(fileName, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
			if err != nil {
				log.Fatalf("failed to reopen results file %s: %v\n", fileName, err)
			}

			func() {
				defer func() {
					if err := f.Close(); err != nil {
						log.Printf("failed to close results file %s: %v\n", fileName, err)
					}
				}()

				wtr := csv.NewWriter(f)

				if err := wtr.Write([]string{}); err != nil {
					log.Fatalf("failed to write blank row to %s: %v\n", fileName, err)
				}

				if err := wtr.Write([]string{c.agentName, ""}); err != nil {
					log.Fatalf("failed to write agent section header to %s: %v\n", fileName, err)
				}

				if err := wtr.Write([]string{"reject_reason", "count"}); err != nil {
					log.Fatalf("failed to write reject-reason header to %s: %v\n", fileName, err)
				}

				rejectReasons := make([]string, 0, len(c.agentRejectReasonToCount))
				for reason := range c.agentRejectReasonToCount {
					rejectReasons = append(rejectReasons, reason)
				}
				sort.Strings(rejectReasons)

				for _, reason := range rejectReasons {
					if err := wtr.Write([]string{
						reason,
						strconv.FormatUint(c.agentRejectReasonToCount[reason], 10),
					}); err != nil {
						log.Fatalf("failed to write reject-reason row to %s: %v\n", fileName, err)
					}
				}

				wtr.Flush()
				if err := wtr.Error(); err != nil {
					log.Fatalf("failed to flush reject-reason csv writer for %s: %v\n", fileName, err)
				}
			}()
		}
	}
}

func connectToTraceDBOrDie(dbFile string, numConns int) (*sql.DB, func()) {
	db, err := sql.Open("sqlite", "file:"+dbFile+"?mode=ro")
	if err != nil {
		log.Fatalf("SIM: Fatal: failed to open sqlite DB file %s storing the trace: %v", dbFile, err)
	}

	db.SetMaxOpenConns(numConns)
	db.SetMaxIdleConns(numConns)

	closeDB := func() {
		if err := db.Close(); err != nil {
			log.Printf("SIM: Failed to close sqlite DB file %s storing the trace: %v\n", dbFile, err)
		}
	}

	return db, closeDB
}

// TODO: fix naming inside the following function: we use "cluster" to mean two completely separate
// things.
// TODO: this function assumes that numWorkers <= num of effective work items. If the assumption
// isn't met it's buggy. But it should be easy to fix that: at some point, do that.
func prepareWorkOrDie(ctx context.Context, db *sql.DB, cfg *Cfg, numWorkers int) ([]work, map[int]struct{}) {
	numWorkItems := numWorkers
	if numWorkers < len(cfg.Clusters) {
		numWorkItems = len(cfg.Clusters)
	}
	workItems := make([]work, len(cfg.Clusters), numWorkItems)

	for i, c := range cfg.Clusters {
		w := work{
			machineID: c.MachineID,
			db:        db,
		}

		if i >= numWorkers-len(cfg.Clusters) {
			// If we're here readers of trace events are also processors of events.
			numClusters := len(c.Specs) * len(cfg.Agents) * len(cfg.HostResources)
			w.clusters = make([]*cluster, numClusters)

			for agentI, agent := range cfg.Agents {
				for specI, spec := range c.Specs {
					for hostResI, hostResources := range cfg.HostResources {
						clusterIdx := agentI*len(c.Specs)*len(cfg.HostResources) + specI*len(cfg.HostResources) + hostResI
						w.clusters[clusterIdx] = newClusterOrDie(ctx, spec.SchedulingAlgo, agent.Name, hostResources.Name, agent, spec.NumHosts, hostResources)
					}
				}
			}
		}

		workItems[i] = w
	}

	// Minimum number of workers that are only processors for each cfg cluster AKA machine ID.
	minNumProcessorsOnly := (numWorkers / len(cfg.Clusters)) - 1

	// All cluster IDs < the following one have one more processor on top of minNumProcessorsOnly.
	minClusterIDWOExtraProcessor := numWorkers % len(cfg.Clusters)

	for i := len(cfg.Clusters); i < numWorkers; i++ {
		clusterID := i % len(cfg.Clusters)

		numProcessorsForThisCluster := minNumProcessorsOnly
		if clusterID < minClusterIDWOExtraProcessor {
			numProcessorsForThisCluster++
		}

		clus := cfg.Clusters[clusterID]
		totNumClusters := len(clus.Specs) * len(cfg.Agents) * len(cfg.HostResources)

		// Now, determine how many clusters this processor must process.
		procIdx := i / len(cfg.Clusters)

		numClustersForThisProcessor := totNumClusters / numProcessorsForThisCluster
		maxProcIdxWExcessCluster := totNumClusters % numProcessorsForThisCluster
		if procIdx <= maxProcIdxWExcessCluster {
			numClustersForThisProcessor++
		}
		startClusterIdx := (procIdx - 1) * numClustersForThisProcessor
		if procIdx > maxProcIdxWExcessCluster {
			startClusterIdx += maxProcIdxWExcessCluster
		}
		endClusterIdx := startClusterIdx + numClustersForThisProcessor

		thisProcClusters := make([]*cluster, numClustersForThisProcessor)

		// TODO: this loop is perversely inefficient, speed it up.
	outer:
		for agentI, agent := range cfg.Agents {
			for specI, spec := range clus.Specs {
				for hostResI, hostResources := range cfg.HostResources {
					clusterIdx := agentI*len(clus.Specs)*len(cfg.HostResources) + specI*len(cfg.HostResources) + hostResI
					if clusterIdx < startClusterIdx {
						continue
					}
					if clusterIdx >= endClusterIdx {
						break outer
					}
					thisProcClusters[clusterIdx-startClusterIdx] = newClusterOrDie(ctx, spec.SchedulingAlgo, agent.Name, hostResources.Name, agent, spec.NumHosts, hostResources)
				}
			}
		}

		ch := make(chan traceEvent, 5)
		w := work{
			machineID: cfg.Clusters[clusterID].MachineID,
			inChan:    ch,
			clusters:  thisProcClusters,
		}
		// wire the out channel for the corresponding reader.
		workItems[clusterID].outChans = append(workItems[clusterID].outChans, ch)

		workItems = append(workItems, w)
	}

	workLeftovers := make(map[int]struct{})
	for i := numWorkers; i < len(cfg.Clusters); i++ {
		workLeftovers[i] = struct{}{}
	}

	return workItems, workLeftovers
}

func newClusterOrDie(ctx context.Context, schedAlgo, agentName, hostType string, agent agentCfg, numHosts int, hostResources hostResourcesCfg) *cluster {
	agents := make([]hostAgent, numHosts)
	hosts := make([]runtime.Object, numHosts)

	hostCPUs := int64(hostResources.NumSockets) * int64(hostResources.SncWays) * int64(hostResources.CpusPerNNUMA)
	hostBytes := int64(hostResources.NumSockets) * int64(hostResources.SncWays) * int64(hostResources.GiBsPerNNUMA) * (1 << 30)

	mi := generateMachineInfo(&hostResources)

	for i := range numHosts {
		agents[i] = newHostAgentOrDie(mi, agent.TopologyManagerPolicy, agent.TopologyManagerOptions)
		hosts[i] = newHost(fmt.Sprintf("host-%d", i), hostBytes, hostCPUs)
	}

	return &cluster{
		schedAlgo:                schedAlgo,
		agentName:                agentName,
		hostType:                 hostType,
		agents:                   agents,
		totHostMemBytes:          hostBytes,
		totHostCPUs:              hostCPUs,
		agentRejectReasonToCount: make(map[string]uint64),
		scheduler:                newSchedulerOrDie(ctx, schedAlgo, hosts),
		admittedVMIDToHost:       make(map[int64]int),
		admittedVMIDToPod:        make(map[int64]*v1.Pod),
	}
}

func newSchedulerOrDie(ctx context.Context, fitStrategy string, hosts []runtime.Object) *scheduler.Scheduler {
	var fitStrategyType schedcfg.ScoringStrategyType
	scoringPlugins := []schedcfg.Plugin{{Name: "NodeResourcesFit", Weight: 1}}
	scoringPluginsCfgs := []schedcfg.PluginConfig{}

	switch strings.ToLower(strings.TrimSpace(fitStrategy)) {
	case "mostallocated":
		fitStrategyType = schedcfg.MostAllocated
	case "leastallocated":
		fitStrategyType = schedcfg.LeastAllocated
		// By default in K8s Least allocated goes together with the NodeResourcesBalancedAllocation
		// plugin and we stick to the defaults as much as possible.
		scoringPlugins = append(scoringPlugins, schedcfg.Plugin{Name: "NodeResourcesBalancedAllocation", Weight: 1})
		scoringPluginsCfgs = append(scoringPluginsCfgs, balanceAllocSchedulingPluginCfg())
	default:
		log.Fatalf("SIM: Received request to create scheduler with fit strategy %s, only supported "+
			"strategies are LeastAllocated and MostAllocated", fitStrategy)
	}
	scoringPluginsCfgs = append(scoringPluginsCfgs, resourceFitSchedulingPluginCfg(fitStrategyType))

	// Load default namespace and cluster nodes in the scheduler in-memory view of the cluster.
	var objs []runtime.Object
	objs = append(objs, &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ""}})
	objs = append(objs, hosts...)
	client := clientsetfake.NewClientset(objs...)
	informerFactory := informers.NewSharedInformerFactory(client, 0)

	broadcaster := events.NewBroadcaster(nopEventSink{})

	nodesToScore := int32(10)
	s, err := scheduler.New(
		ctx,
		client,
		informerFactory,
		nil,
		profile.NewRecorderFactory(broadcaster),
		scheduler.WithPercentageOfNodesToScore(&nodesToScore),
		scheduler.WithProfiles(
			// We don't want to be attacked on the choice of the scheduling policy. So we pass the
			// "default" K8s scheduler, i.e., enable all default plugins. Note that we don't
			// actually pass all default plugins, because some are out of scope. i.e. in our
			// experiments pods only request CPUs and memory, not storage, so we disable all plugins
			// dealing with storage volumes are disabled. QueueSort and Bind plugins we actually
			// don't need, but scheduler.New(...) fails without them.
			schedcfg.KubeSchedulerProfile{SchedulerName: miniDefaultK8sScheduler,
				Plugins: &schedcfg.Plugins{
					QueueSort: schedcfg.PluginSet{Enabled: []schedcfg.Plugin{{Name: "PrioritySort"}}},
					Bind:      schedcfg.PluginSet{Enabled: []schedcfg.Plugin{{Name: "DefaultBinder"}}},
					PreFilter: schedcfg.PluginSet{Enabled: []schedcfg.Plugin{{Name: "NodeResourcesFit"}}},
					Filter:    schedcfg.PluginSet{Enabled: []schedcfg.Plugin{{Name: "NodeResourcesFit"}}},
					Score:     schedcfg.PluginSet{Enabled: scoringPlugins},
				},
				PluginConfig: scoringPluginsCfgs,
			},
		),
	)
	if err != nil {
		log.Fatalf("SIM: Failed to instantiate K8s scheduler: %v\n", err)
	}

	informerFactory.Start(ctx.Done())
	informerFactory.WaitForCacheSync(ctx.Done())

	return s
}

func balanceAllocSchedulingPluginCfg() schedcfg.PluginConfig {
	return schedcfg.PluginConfig{
		Name: "NodeResourcesBalancedAllocation",
		Args: &schedcfg.NodeResourcesBalancedAllocationArgs{
			Resources: []schedcfg.ResourceSpec{
				{Name: "cpu", Weight: 1},
				{Name: "memory", Weight: 1},
			},
		},
	}
}

func resourceFitSchedulingPluginCfg(fitType schedcfg.ScoringStrategyType) schedcfg.PluginConfig {
	return schedcfg.PluginConfig{
		Name: "NodeResourcesFit",
		Args: &schedcfg.NodeResourcesFitArgs{
			ScoringStrategy: &schedcfg.ScoringStrategy{
				Type: fitType,
				Resources: []schedcfg.ResourceSpec{
					{Name: "cpu", Weight: 1},
					{Name: "memory", Weight: 1},
				},
			},
		},
	}
}

var _ events.EventSink = nopEventSink{}

// K8s fake event sink for testing records all events. It's not a viable option for us, because in
// our simulation traces we might have a lot of events, so memory footprint might be a problem.
// Since we don't care about events, our fake sink discards them silently.
type nopEventSink struct{}

func (n nopEventSink) Create(_ context.Context, e *eventsv1.Event) (*eventsv1.Event, error) {
	return e, nil
}

func (n nopEventSink) Patch(_ context.Context, e *eventsv1.Event, _ []byte) (*eventsv1.Event, error) {
	return e, nil
}

func (n nopEventSink) Update(_ context.Context, e *eventsv1.Event) (*eventsv1.Event, error) {
	return e, nil
}

// NOTE: this code assumes the topology is either the one in VLAB, or the one in S4. It does not
// generalize to arbitrary topologies.
func generateMachineInfo(cfg *hostResourcesCfg) *cadvisorapi.MachineInfo {
	// By "Cores" we mean physical cores, while cadvisor means hyperthreads.
	numCores := cfg.NumSockets * cfg.SncWays * cfg.CpusPerNNUMA / threadsPerCore
	nodes := make([]cadvisorapi.Node, cfg.NumSockets*cfg.SncWays)

	perNNUMABytes := uint64(cfg.GiBsPerNNUMA) * (1 << 30)

	// sockID and nodeID are global, while coreID and thdID are per-node and per-core, respectively.
	for sockID := range cfg.NumSockets {
		for i := range cfg.SncWays {
			nodeID := sockID*cfg.SncWays + i
			cores := make([]cadvisorapi.Core, cfg.CpusPerNNUMA/threadsPerCore)
			for coreID := range cores {
				cores[coreID].Id = coreID
				cores[coreID].SocketID = sockID
				cores[coreID].Threads = make([]int, threadsPerCore)
				for thdID := range threadsPerCore {
					cores[coreID].Threads[thdID] = nodeID*len(cores) + coreID + thdID*numCores
				}
			}
			nodes[nodeID] = cadvisorapi.Node{
				Id:     nodeID,
				Cores:  cores,
				Memory: perNNUMABytes,
			}
		}
	}

	// Set NUMA distances.
	for i := range nodes {
		d := make([]uint64, len(nodes))
		for j := range nodes {
			if j == i {
				d[j] = intraNUMADistance
			} else if nodes[i].Cores[0].SocketID == nodes[j].Cores[0].SocketID {
				d[j] = intraSocketDistance
			} else {
				d[j] = outOfSocketDistance
			}
		}
		nodes[i].Distances = d
	}

	return &cadvisorapi.MachineInfo{
		Topology:         nodes,
		NumSockets:       cfg.NumSockets,
		NumPhysicalCores: numCores,
		// cadvisor confusingly uses "cores" to mean "threads" here.
		NumCores:       numCores * threadsPerCore,
		MemoryCapacity: uint64(len(nodes)) * perNNUMABytes,
	}
}
