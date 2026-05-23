//go:build stinglet

package main

import (
	"fmt"
	"log"
	"time"

	cadvisorapi "github.com/google/cadvisor/info/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/kubernetes/pkg/kubelet/apis/config"
	"k8s.io/kubernetes/pkg/kubelet/cm/farmemtopologymanager"
	"k8s.io/kubernetes/pkg/kubelet/lifecycle"
	"k8s.io/utils/cpuset"
)

type stinglet struct {
	mgr          *farmemtopologymanager.Manager
	admittedPods []*v1.Pod
}

var _ hostAgent = &stinglet{}

func (s *stinglet) Admit(p *v1.Pod) error {
	res := s.mgr.Admit(&lifecycle.PodAdmitAttributes{
		Pod: p,
	})

	if !res.Admit {
		return fmt.Errorf("Pod rejected by agent. Reason: %s Message: %s", res.Reason, res.Message)
	}

	s.admittedPods = append(s.admittedPods, p)

	return nil
}

func (s *stinglet) HandleDeparture(podName string) error {
	for i, p := range s.admittedPods {
		if p.Name == podName {
			if err := s.mgr.SimRemoveContainer(string(p.UID), p.Spec.Containers[0].Name); err != nil {
				return fmt.Errorf("failed to remove pod %s: %v", podName, err)
			}
			s.admittedPods[i] = s.admittedPods[len(s.admittedPods)-1]
			// Set to nil, otherwise the GC keeps it around.
			s.admittedPods[len(s.admittedPods)-1] = nil
			s.admittedPods = s.admittedPods[:len(s.admittedPods)-1]
			return nil
		}
	}
	return nil
}

func newHostAgentOrDie(mi *cadvisorapi.MachineInfo, _ string, _ map[string]string) *stinglet {
	reservedCPUs := cpuset.New(mi.Topology[0].Cores[0].Threads...)
	resMemory4KiB := int64(1 << 12)
	reservations := v1.ResourceList{
		v1.ResourceCPU: *resource.NewMilliQuantity(int64(reservedCPUs.Size())*1000,
			resource.DecimalSI),
		v1.ResourceMemory: *resource.NewQuantity(resMemory4KiB, resource.DecimalSI),
	}
	perNUMANodeResMem := []config.MemoryReservation{
		{
			// We assume there's at least one NUMA node, and that it has index 0.
			NumaNode: 0,
			Limits:   reservations,
		},
	}

	// Same as K8s default, although it doesn't really matter because we're not doing a
	// reconciliation here.
	defaultReconcilePeriod := 10 * time.Second

	m, err := farmemtopologymanager.New(mi, reservedCPUs, perNUMANodeResMem, reservations,
		defaultReconcilePeriod)
	if err != nil {
		log.Fatalf("Failed to create stinglet-based manager: %v\n", err)
	}

	return &stinglet{
		mgr:          m,
		admittedPods: make([]*v1.Pod, 0, 8),
	}
}
