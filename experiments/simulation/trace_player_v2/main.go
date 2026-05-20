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
		// TODO: we don't want the default!
		scheduler.WithProfiles(
			schedulerapi.KubeSchedulerProfile{
				SchedulerName: "default-scheduler",
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
