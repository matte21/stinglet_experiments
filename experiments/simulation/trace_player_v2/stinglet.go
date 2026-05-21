//go:build stinglet

package main

import v1 "k8s.io/api/core/v1"

type stinglet struct{}

var _ hostAgent = &stinglet{}

func (s *stinglet) Admit(*v1.Pod) error {
	panic("unimplemented")
}

func newHostAgent() *stinglet {
	return &stinglet{}
}
