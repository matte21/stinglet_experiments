//go:build !stinglet

package main

import (
	v1 "k8s.io/api/core/v1"
	"k8s.io/kubernetes/pkg/kubelet/cm/topologymanager"
)

type stdKubelet struct {
	topoMgr topologymanager.Manager
}

var _ hostAgent = &stdKubelet{}

func (s *stdKubelet) Admit(*v1.Pod) error {
	panic("unimplemented")
}

func newHostAgent() *stdKubelet {
	return &stdKubelet{}
}
