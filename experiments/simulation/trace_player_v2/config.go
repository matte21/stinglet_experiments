package main

type clusterCfg struct {
	NumHosts int `yaml:"numberOfHosts"`
	// "LeastAllocated" or "MostAllocated"
	SchedulingAlgo string `yaml:"schedulingAlgorithm"`
	MachineID      int    `yaml:"machineId"`
}

// Note: currently we assume that the interconnect between the Sockets is a full mesh.
// We also assume 1 socket = 1 physical NUMA node.
type hostResourcesCfg struct {
	NumSockets int `yaml:"numberOfSockets"`
	SncWays    int `yaml:"sncWays"`

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

type cfg struct {
	// At least one is required.
	Clusters []clusterCfg `yaml:"clusters"`

	// At least one is required.
	HostResources []hostResourcesCfg `yaml:"hostResources"`

	// At least one required, except when the agent is stinglet (in that case none is required).
	Agents []agentCfg `yaml:"agents"`
}
