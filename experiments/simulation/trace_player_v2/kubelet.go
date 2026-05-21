//go:build !stinglet

package main

import v1 "k8s.io/api/core/v1"

type stdKubelet struct{}

var _ hostAgent = &stdKubelet{}

func (s *stdKubelet) Admit(*v1.Pod) error {
	panic("unimplemented")
}

func newHostAgent() *stdKubelet {
	return &stdKubelet{}
}
