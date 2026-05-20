package main

import (
	"fmt"

	_ "k8s.io/kubernetes/pkg/scheduler"
)

var Version = "dev"

func main() {
	fmt.Println(Version)
}
