package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	cadvisorapi "github.com/google/cadvisor/info/v1"
	v1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/informers"
	clientsetfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/events"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/kubelet/cm/topologymanager"
	"k8s.io/kubernetes/pkg/scheduler"
	schedcfg "k8s.io/kubernetes/pkg/scheduler/apis/config"
	"k8s.io/kubernetes/pkg/scheduler/profile"
)

// TODO: consider using a context with cancel for the scheduler since we plan to reuse the
// same scheduler to play multiple independent traces.

const miniDefaultK8sScheduler = "mini-default-k8s-sched"

var Version = "dev"

type hostAgent interface {
	Admit(*v1.Pod) error
	HandleDeparture(podName string) error
}

// Note: currently we assume that SMT is enabled with two threads per core.
const (
	threadsPerCore = 2
	// The following values are taken from S4.
	intraNUMADistance   = 10
	intraSocketDistance = 12
	outOfSocketDistance = 21
)

// Note: currently we assume that the interconnect between the Sockets is a full mesh.
// We also assume 1 socket = 1 physical NUMA node.
type simCfg struct {
	numSockets int
	sncWays    int

	// if sncWays = 1, this is the # of CPUs on a physical NUMA node. If sncWays > 1, this is the
	// number of CPUs on a logical nNUMA, i.e. a sub NUMA cluster.
	cpusPerNNUMA int

	// if sncWays = 1, this is the # of GiBs on a physical NUMA node. If sncWays > 1, this is the
	// number of GiBs on a logical nNUMA, i.e. a subNUMA.
	gibsPerNNUMA int
}

func main() {
	log.Printf("Running %s\n", Version)

	//testCaseTopoParsingAndStinglet2()
	//testCaseTopoParsingAndKubelets1()
	//testCaseTopoParsingAndKubelets2()

	//testCaseTopoParsing1()
	//testCaseTopoParsing2()

	//! rm when done
	s := newSchedulerOrDie(2, (1 << 33), 64*1000, "MostAllocated")
	//s := newSchedulerOrDie(2, (1 << 33), 64*1000, "LeastAllocated")
	testCase1(s)
	//testCase2(s)

	//! rm when done
	//_ = newHostAgent()
}

func newSchedulerOrDie(numHosts uint32, hostBytes, hostMilliCPUs int64, fitStrategy string) *scheduler.Scheduler {
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
		log.Fatalf("Received request to create scheduler with fit strategy %s, only supported "+
			"strategies are LeastAllocated and MostAllocated", fitStrategy)
	}
	scoringPluginsCfgs = append(scoringPluginsCfgs, resourceFitSchedulingPluginCfg(fitStrategyType))

	// Load default namespace and cluster nodes in the scheduler in-memory view of the cluster.
	var objs []runtime.Object
	objs = append(objs, &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ""}})
	// TODO: replace with nodes loaded from trace
	for i := 0; i < int(numHosts); i++ {
		name := fmt.Sprintf("node-%d", i)
		objs = append(objs, newHost(name, hostBytes, hostMilliCPUs))
	}
	client := clientsetfake.NewClientset(objs...)
	informerFactory := informers.NewSharedInformerFactory(client, 0)

	broadcaster := events.NewBroadcaster(nopEventSink{})
	ctx := context.Background()

	s, err := scheduler.New(
		ctx,
		client,
		informerFactory,
		nil,
		profile.NewRecorderFactory(broadcaster),
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
		log.Fatalf("Failed to instantiate K8s scheduler: %v\n", err)
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

func newHost(name string, bytes, milliCPUs int64) *v1.Node {
	return &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			UID:  types.UID(name),
		},
		Status: v1.NodeStatus{
			Capacity: v1.ResourceList{
				v1.ResourceCPU:    *resource.NewMilliQuantity(milliCPUs, resource.DecimalSI),
				v1.ResourceMemory: *resource.NewQuantity(bytes, resource.BinarySI),
			},
			Allocatable: v1.ResourceList{
				v1.ResourceCPU:    *resource.NewMilliQuantity(milliCPUs, resource.DecimalSI),
				v1.ResourceMemory: *resource.NewQuantity(bytes, resource.BinarySI),
				v1.ResourcePods:   *resource.NewQuantity(110, resource.DecimalSI),
			},
		},
	}
}

// Note: CPUs unit is whole cores, not millicores.
func newPod(name string, bytes, cpusCores int64, guaranteed bool) *v1.Pod {
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
							v1.ResourceCPU:    *resource.NewMilliQuantity(cpusCores*1000, resource.DecimalSI),
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

// TODO: remove me.
func testCase1(s *scheduler.Scheduler) {
	allPods := []*v1.Pod{
		newPod("pod-1", 4*(1<<30), 32, true),
		newPod("pod-2", 4*(1<<30), 32, true),
		newPod("pod-3", 5*(1<<30), 32, true),
	}

	for _, p := range allPods {
		o, err := s.SimSchedulePod(context.Background(), p)
		if err != nil {
			log.Printf("Failed to schedule pod %s: %v\n", p.Name, err)
			continue
		}
		log.Printf("Pod %s to %s\n", p.Name, o.SuggestedHost)
		p.Spec.NodeName = o.SuggestedHost
		s.Cache.AddPod(klog.Background(), p)
	}

	s.Cache.RemovePod(klog.Background(), allPods[0])

	p := newPod("pod-4", 5*(1<<30), 32, true)
	o, err := s.SimSchedulePod(context.Background(), p)
	if err != nil {
		log.Printf("Failed to schedule pod %s: %v\n", p.Name, err)
		return
	}
	log.Printf("Pod %s to %s\n", p.Name, o.SuggestedHost)
	p.Spec.NodeName = o.SuggestedHost
	s.Cache.AddPod(klog.Background(), p)
}

// TODO: remove me.
func testCase2(s *scheduler.Scheduler) {
	allPods := []*v1.Pod{
		newPod("pod-1", 2*(1<<30), 32, true),
		newPod("pod-2", 2*(1<<30), 32, true),
	}

	for _, p := range allPods {
		o, err := s.SimSchedulePod(context.Background(), p)
		if err != nil {
			log.Printf("Failed to schedule pod %s: %v\n", p.Name, err)
			continue
		}
		log.Printf("Pod %s to %s\n", p.Name, o.SuggestedHost)
		p.Spec.NodeName = o.SuggestedHost
		s.Cache.AddPod(klog.Background(), p)
	}
}

// NOTE: this code assumes the topology is either the one in VLAB, or the one in S4. It does not
// generalize to arbitrary topologies.
func generateMachineInfo(cfg *simCfg) *cadvisorapi.MachineInfo {
	// By "Cores" we mean physical cores, while cadvisor means hyperthreads.
	numCores := cfg.numSockets * cfg.sncWays * cfg.cpusPerNNUMA / threadsPerCore
	nodes := make([]cadvisorapi.Node, cfg.numSockets*cfg.sncWays)

	perNNUMABytes := uint64(cfg.gibsPerNNUMA) * (1 << 30)

	// sockID and nodeID are global, while coreID and thdID are per-node and per-core, respectively.
	for sockID := range cfg.numSockets {
		for i := range cfg.sncWays {
			nodeID := sockID*cfg.sncWays + i
			cores := make([]cadvisorapi.Core, cfg.cpusPerNNUMA/threadsPerCore)
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
		NumSockets:       cfg.numSockets,
		NumPhysicalCores: numCores,
		// cadvisor confusingly uses "cores" to mean "threads" here.
		NumCores:       numCores * threadsPerCore,
		MemoryCapacity: uint64(len(nodes)) * perNNUMABytes,
	}
}

// TODO: rm me.
func testCaseTopoParsing1() {
	sc := &simCfg{
		numSockets:   2,
		sncWays:      1,
		cpusPerNNUMA: 64,
		gibsPerNNUMA: 256,
	}

	t := generateMachineInfo(sc)

	fmt.Println(t)
}

// TODO: rm me.
func testCaseTopoParsing2() {
	sc := &simCfg{
		numSockets:   2,
		sncWays:      2,
		cpusPerNNUMA: 32,
		gibsPerNNUMA: 128,
	}

	t := generateMachineInfo(sc)

	fmt.Println(t)
}

// TODO: rm me.
func testCaseTopoParsingAndKubelets1() {
	sc := &simCfg{
		numSockets:   2,
		sncWays:      2,
		cpusPerNNUMA: 32,
		gibsPerNNUMA: 128,
	}

	t := generateMachineInfo(sc)

	agent := newHostAgentOrDie(t, topologymanager.PolicyBestEffort, nil)

	allPods := []*v1.Pod{
		newPod("pod-1", int64((sc.gibsPerNNUMA*3)/4)*(1<<30), 2, true),
		newPod("pod-2", int64((sc.gibsPerNNUMA*3)/4)*(1<<30), 2, true),
		newPod("pod-3", int64((sc.gibsPerNNUMA*3)/4)*(1<<30), 2, true),
		newPod("pod-4", int64((sc.gibsPerNNUMA*3)/4)*(1<<30), 2, true),
	}

	for _, p := range allPods {
		if err := agent.Admit(p); err != nil {
			fmt.Printf("agent rejected pod %s: %v\n", p.Name, err)
		} else {
			fmt.Printf("agent accepted pod %s\n", p.Name)
		}
	}

	rejectPod := newPod("pod-5", int64(sc.gibsPerNNUMA/2)*(1<<30), 2, true)
	if err := agent.Admit(rejectPod); err != nil {
		fmt.Printf("agent rejected pod %s: %v\n", rejectPod.Name, err)
	} else {
		fmt.Printf("agent accepted pod %s\n", rejectPod.Name)
	}

	agent.HandleDeparture("pod-1")

	acceptedPod := newPod("pod-6", int64(sc.gibsPerNNUMA/2)*(1<<30), 2, true)
	if err := agent.Admit(acceptedPod); err != nil {
		fmt.Printf("agent rejected pod %s: %v\n", acceptedPod.Name, err)
	} else {
		fmt.Printf("agent accepted pod %s\n", acceptedPod.Name)
	}
}

// TODO: rm me.
func testCaseTopoParsingAndKubelets2() {
	sc := &simCfg{
		numSockets:   2,
		sncWays:      2,
		cpusPerNNUMA: 32,
		gibsPerNNUMA: 128,
	}

	t := generateMachineInfo(sc)

	agentDefault := newHostAgentOrDie(t, topologymanager.PolicyBestEffort, nil)
	agentClose := newHostAgentOrDie(t, topologymanager.PolicyBestEffort, map[string]string{
		topologymanager.PreferClosestNUMANodes: "true",
	})

	allPods := []*v1.Pod{
		newPod("pod-1", int64(sc.gibsPerNNUMA-10)*(1<<30), 2, true),
		newPod("pod-2", int64(sc.gibsPerNNUMA*2)*(1<<30), 2, true),
	}

	for _, p := range allPods {
		if err := agentDefault.Admit(p); err != nil {
			fmt.Printf("agent default rejected pod %s: %v\n", p.Name, err)
		} else {
			fmt.Printf("agent default accepted pod %s\n", p.Name)
		}

		if err := agentClose.Admit(p); err != nil {
			fmt.Printf("agent close rejected pod %s: %v\n", p.Name, err)
		} else {
			fmt.Printf("agent close accepted pod %s\n", p.Name)
		}
	}
}

// TODO: rm me.
func testCaseTopoParsingAndStinglet1() {
	sc := &simCfg{
		numSockets:   2,
		sncWays:      2,
		cpusPerNNUMA: 32,
		gibsPerNNUMA: 128,
	}

	t := generateMachineInfo(sc)

	agent := newHostAgentOrDie(t, "", nil)

	allPods := []*v1.Pod{
		newPod("pod-1", int64((sc.gibsPerNNUMA*3)/4)*(1<<30), 2, true),
		newPod("pod-2", int64((sc.gibsPerNNUMA*3)/4)*(1<<30), 2, true),
		newPod("pod-3", int64((sc.gibsPerNNUMA*3)/4)*(1<<30), 2, true),
		newPod("pod-4", int64((sc.gibsPerNNUMA*3)/4)*(1<<30), 2, true),
		newPod("pod-5", int64((sc.gibsPerNNUMA*3)/4-5)*(1<<30), 2, true),
	}

	for _, p := range allPods {
		if err := agent.Admit(p); err != nil {
			fmt.Printf("agent rejected pod %s: %v\n", p.Name, err)
		} else {
			fmt.Printf("agent accepted pod %s\n", p.Name)
		}
	}

	fmt.Println("foo")
}

// TODO: rm me.
func testCaseTopoParsingAndStinglet2() {
	sc := &simCfg{
		numSockets:   2,
		sncWays:      2,
		cpusPerNNUMA: 32,
		gibsPerNNUMA: 128,
	}

	t := generateMachineInfo(sc)

	agent := newHostAgentOrDie(t, "", nil)

	allPods := []*v1.Pod{
		newPod("pod-1", int64((sc.gibsPerNNUMA*3)/4)*(1<<30), 2, true),
		newPod("pod-2", int64((sc.gibsPerNNUMA*3)/4)*(1<<30), 2, true),
		newPod("pod-3", int64(sc.gibsPerNNUMA*2)*(1<<30), 2, true),
	}

	for _, p := range allPods {
		if err := agent.Admit(p); err != nil {
			fmt.Printf("agent rejected pod %s: %v\n", p.Name, err)
		} else {
			fmt.Printf("agent accepted pod %s\n", p.Name)
		}
	}

	for _, p := range allPods {
		if err := agent.HandleDeparture(p.Name); err != nil {
			fmt.Printf("agent failed to delete pod %s: %v\n", p.Name, err)
		} else {
			fmt.Printf("agent deleted pod %s\n", p.Name)
		}
	}

}
