package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type clusterSpec struct {
	NumHosts int `yaml:"numberOfHosts"`
	// "LeastAllocated" or "MostAllocated"
	SchedulingAlgo string `yaml:"schedulingAlgorithm"`
}

type clusterCfg struct {
	Specs     []clusterSpec `yaml:"specs"`
	MachineID int           `yaml:"machineId"`
}

// Note: currently we assume that the interconnect between the Sockets is a full mesh.
// We also assume 1 socket = 1 physical NUMA node.
type hostResourcesCfg struct {
	Name       string `yaml:"name"`
	NumSockets int    `yaml:"numberOfSockets"`
	SncWays    int    `yaml:"sncWays"`

	// if sncWays = 1, this is the # of CPUs on a physical NUMA node. If sncWays > 1, this is the
	// number of CPUs on a logical nNUMA, i.e. a sub NUMA cluster.
	CpusPerNNUMA int `yaml:"cpusPerNNUMA"`

	// if sncWays = 1, this is the # of GiBs on a physical NUMA node. If sncWays > 1, this is the
	// number of GiBs on a logical nNUMA, i.e. a sub NUMA cluster.
	GiBsPerNNUMA int `yaml:"gibsPerNNUMA"`
}

type agentCfg struct {
	// currently only "none" and "best-effort" are supported.
	TopologyManagerPolicy string `yaml:"topologyManagerPolicy"`

	// e.g. prefer-closest-numa-nodes: "true"
	TopologyManagerOptions map[string]string `yaml:"topologyManagerOptions"`
}

type Cfg struct {
	// At least one is required.
	Clusters []clusterCfg `yaml:"clusters"`

	// At least one is required.
	HostResources []hostResourcesCfg `yaml:"hostResources"`

	// At least one required, except when the agent is stinglet (in that case none is required).
	Agents []agentCfg `yaml:"agents"`
}

func parseCfgOrDie(cfgFilePath string) *Cfg {
	c, err := parseCfg(cfgFilePath)
	if err != nil {
		log.Fatalf("SIM: Fatal: %v", err)
	}
	return c
}

func parseCfg(cfgFilePath string) (*Cfg, error) {
	data, err := os.ReadFile(cfgFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", cfgFilePath, err)
	}

	var c Cfg
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("failed to parse contents of config file %s: %w", cfgFilePath, err)
	}

	if err := validateCfg(&c); err != nil {
		return nil, fmt.Errorf("config in file %s is invalid: %w", cfgFilePath, err)
	}

	return &c, nil
}

func validateCfg(c *Cfg) error {
	if len(c.Clusters) == 0 {
		return fmt.Errorf("at least one cluster config is required")
	}
	if len(c.HostResources) == 0 {
		return fmt.Errorf("at least one hostResources config is required")
	}

	for _, cl := range c.Clusters {
		if len(cl.Specs) == 0 {
			return fmt.Errorf("each cluster must have at least one spec (scheduling algorithm + number of hosts), cluster machineId=%d has none", cl.MachineID)
		}
		for i, s := range cl.Specs {
			if s.NumHosts <= 0 {
				return fmt.Errorf("cluster machineId=%d, spec=%d: numberOfHosts must be > 0", cl.MachineID, i)
			}
			schedAlgo := strings.ToLower(strings.TrimSpace(s.SchedulingAlgo))
			if schedAlgo != "leastallocated" && schedAlgo != "mostallocated" {
				return fmt.Errorf("cluster machineId=%d, spec=%d: unknown schedulingAlgorithm %q, "+
					"must be LeastAllocated or MostAllocated", cl.MachineID, i, s.SchedulingAlgo)
			}
		}
	}

	for i, hr := range c.HostResources {
		if hr.Name == "" {
			return fmt.Errorf("hostResources entry %d is missing a name (mandatory)", i)
		}
		if hr.NumSockets <= 0 {
			return fmt.Errorf("hostResources[%s]: numSockets must be > 0", hr.Name)
		}
		if hr.SncWays <= 0 {
			return fmt.Errorf("hostResources[%s]: sncWays must be > 0", hr.Name)
		}
		if hr.CpusPerNNUMA <= 0 {
			return fmt.Errorf("hostResources[%s]: cpusPerNNUMA must be > 0", hr.Name)
		}
		if hr.GiBsPerNNUMA <= 0 {
			return fmt.Errorf("hostResources[%s]: gibsPerNNUMA must be > 0", hr.Name)
		}
	}

	// Validated separately per build tag in the agent-specific files.
	return validateAgentCfgs(c.Agents)
}
