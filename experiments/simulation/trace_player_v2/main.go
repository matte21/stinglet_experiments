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
	schedulerapi "k8s.io/kubernetes/pkg/scheduler/apis/config"
	"k8s.io/kubernetes/pkg/scheduler/profile"
)

// TODO: consider using a context with cancel for the scheduler since we plan to reuse the
// same scheduler to play multiple independent traces.

const miniDefaultK8sScheduler = "mini-default-k8s-sched"

var Version = "dev"

func main() {
	log.Printf("Running %s\n", Version)

	_ = newSchedulerOrDie(2, (1 << 33), 64*1000)
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
			// dealing with storage volumes are disabled.
			schedulerapi.KubeSchedulerProfile{SchedulerName: miniDefaultK8sScheduler,
				Plugins: &schedulerapi.Plugins{
					PreFilter: schedulerapi.PluginSet{Enabled: []schedulerapi.Plugin{{Name: "NodeResourcesFit"}}},
					Filter:    schedulerapi.PluginSet{Enabled: []schedulerapi.Plugin{{Name: "NodeResourcesFit"}}},
					Score: schedulerapi.PluginSet{Enabled: []schedulerapi.Plugin{
						{Name: "NodeResourcesFit"},
						{Name: "NodeResourcesBalancedAllocation"},
					}},
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
