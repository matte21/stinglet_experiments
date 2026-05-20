package main

import (
	"fmt"

	"k8s.io/kubernetes/pkg/scheduler"
)

var Version = "dev"

func main() {
	fmt.Println(Version)
	foo()
	s := &scheduler.Scheduler{}
	s.SimSchedulePod()
}
