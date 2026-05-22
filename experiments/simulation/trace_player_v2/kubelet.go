//go:build !stinglet

package main

import (
	"fmt"
	"log"
	"time"

	cadvisorapi "github.com/google/cadvisor/info/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/kubernetes/pkg/kubelet/apis/config"
	"k8s.io/kubernetes/pkg/kubelet/cm/cpumanager"
	"k8s.io/kubernetes/pkg/kubelet/cm/memorymanager"
	"k8s.io/kubernetes/pkg/kubelet/cm/topologymanager"
	"k8s.io/kubernetes/pkg/kubelet/lifecycle"
	"k8s.io/utils/cpuset"
)

type stdKubelet struct {
	topoMgr topologymanager.Manager
	cpuMgr  cpumanager.Manager
	memMgr  memorymanager.Manager
}

var _ hostAgent = &stdKubelet{}

// TODO: measure admit time!
func (s *stdKubelet) Admit(p *v1.Pod) error {
	res := s.topoMgr.Admit(&lifecycle.PodAdmitAttributes{
		Pod: p,
	})

	if res.Admit {
		return nil
	}

	return fmt.Errorf("Pod rejected by agent. Reason: %s Message: %s", res.Reason, res.Message)
}

func newHostAgentOrDie(mi *cadvisorapi.MachineInfo,
	topoMgrPolicy string, topoMgrPolicyOpts map[string]string) *stdKubelet {

	// K8s default.
	mgrsStateFileDir := "/var/lib/kubelet"

	tm := newTopologyMgrOrDie(mi.Topology, topoMgrPolicy, topoMgrPolicyOpts)
	cm := newCPUMgrOrDie(tm, mi, mgrsStateFileDir)
	mm := newMemMgrOrDie(tm, mi, mgrsStateFileDir)

	tm.AddHintProvider(cm)
	tm.AddHintProvider(mm)

	return &stdKubelet{
		topoMgr: tm,
		cpuMgr:  cm,
		memMgr:  mm,
	}
}

func newTopologyMgrOrDie(topology []cadvisorapi.Node, policy string,
	policyOpts map[string]string) topologymanager.Manager {

	topoMgrContainerScope := "container"
	tm, err := topologymanager.NewManager(topology, policy, topoMgrContainerScope, policyOpts)
	if err != nil {
		log.Fatalf("Failed to create topology manager: %v\n", err)
	}
	return tm
}

func newCPUMgrOrDie(tm topologymanager.Manager,
	mi *cadvisorapi.MachineInfo, stateFileDir string) cpumanager.Manager {

	policy := "static"
	policyOpts := map[string]string{
		cpumanager.FullPCPUsOnlyOption: "true",
	}

	// Use K8s default period.
	cpuMgrReconcilePeriod := 10 * time.Second

	reservedCPUs := cpuset.New(mi.Topology[0].Cores[0].Threads...)
	cpusReservation := v1.ResourceList{
		v1.ResourceCPU: *resource.NewMilliQuantity(int64(reservedCPUs.Size())*1000,
			resource.DecimalSI),
	}

	cm, err := cpumanager.NewManager(policy, policyOpts, cpuMgrReconcilePeriod, mi,
		reservedCPUs, cpusReservation, stateFileDir, tm)
	if err != nil {
		log.Fatalf("Failed to create CPU manager: %v\n", err)
	}

	return cm
}

func newMemMgrOrDie(tm topologymanager.Manager, mi *cadvisorapi.MachineInfo,
	stateFileDir string) memorymanager.Manager {

	policy := "Static"

	resMemory4KiB := int64(1 << 12)
	memReservation := v1.ResourceList{
		v1.ResourceMemory: *resource.NewQuantity(resMemory4KiB, resource.DecimalSI),
	}
	perNUMANodeResMem := []config.MemoryReservation{
		{
			// We assume there's at least one NUMA node, and that it has index 0.
			NumaNode: 0,
			Limits:   memReservation.DeepCopy(),
		},
	}

	mm, err := memorymanager.NewManager(policy, mi, memReservation, perNUMANodeResMem,
		stateFileDir, tm)
	if err != nil {
		log.Fatalf("Failed to create memory manager: %v\n", err)
	}

	return mm
}
