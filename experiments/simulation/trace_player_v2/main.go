package main

import (
	"context"
	"fmt"
	"log"

	v1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/informers"
	clientsetfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/events"
	"k8s.io/kubernetes/pkg/scheduler"
	schedcfg "k8s.io/kubernetes/pkg/scheduler/apis/config"
	"k8s.io/kubernetes/pkg/scheduler/profile"
)

// TODO: consider using a context with cancel for the scheduler since we plan to reuse the
// same scheduler to play multiple independent traces.

const miniDefaultK8sScheduler = "mini-default-k8s-sched"

var Version = "dev"

func main() {
	log.Printf("Running %s\n", Version)

	s := newSchedulerOrDie(2, (1 << 33), 64*1000)

	p := newPod("pod-1", 4, 32, true)
	s.SimSchedulePod(context.Background(), p)
}

func newSchedulerOrDie(numNodes uint32, hostMilliCPUs, hostBytes int64) *scheduler.Scheduler {
	ctx := context.Background()

	// Load default namespace and cluster nodes in the scheduler in-memory view of the cluster.
	var objs []runtime.Object
	objs = append(objs, &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ""}})
	// TODO: replace with nodes loaded from trace
	for i := 0; i < int(numNodes); i++ {
		name := fmt.Sprintf("node-%d", i)
		objs = append(objs, newHost(name, hostBytes, hostMilliCPUs))
	}

	client := clientsetfake.NewClientset(objs...)
	broadcaster := events.NewBroadcaster(nopEventSink{})
	informerFactory := informers.NewSharedInformerFactory(client, 0)

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
					Score: schedcfg.PluginSet{Enabled: []schedcfg.Plugin{
						{Name: "NodeResourcesFit"},
						{Name: "NodeResourcesBalancedAllocation"},
					}},
				},
				PluginConfig: []schedcfg.PluginConfig{
					{
						Name: "NodeResourcesFit",
						Args: &schedcfg.NodeResourcesFitArgs{
							ScoringStrategy: &schedcfg.ScoringStrategy{
								// TODO: make the actual policy pluggable.
								Type: schedcfg.LeastAllocated,
								Resources: []schedcfg.ResourceSpec{
									{Name: "cpu", Weight: 1},
									{Name: "memory", Weight: 1},
								},
							},
						},
					},
					{
						Name: "NodeResourcesBalancedAllocation",
						Args: &schedcfg.NodeResourcesBalancedAllocationArgs{
							Resources: []schedcfg.ResourceSpec{
								{Name: "cpu", Weight: 1},
								{Name: "memory", Weight: 1},
							},
						},
					},
				},
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
