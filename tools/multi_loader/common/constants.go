package common

const (
	TraceFormatString = "{}"
)

// Multi-loader possible collectable metrics
const (
	Activator  string = "activator"
	AutoScaler string = "autoscaler"
	TOP        string = "top"
	Prometheus string = "prometheus"
)

var ValidCollectableMetrics = []string{Activator, AutoScaler, TOP, Prometheus}

// Node Types Enum
type NodeType int

const (
	Worker NodeType = iota
	Master
	Loader
	SingleNode
)

var NodeTypeStrings = []string{
	Worker:     "worker",
	Master:     "master",
	Loader:     "monitoring",
	SingleNode: "singlenode",
}

func (n NodeType) String() string {
	return NodeTypeStrings[n]
}

// Pod Types Enum
type PodType int

const (
	ActivatorPod PodType = iota
	AutoScalerPod
)

var PodTypeStrings = []string{
	ActivatorPod:  "activator",
	AutoScalerPod: "autoscaler",
}

func (p PodType) String() string {
	return PodTypeStrings[p]
}
