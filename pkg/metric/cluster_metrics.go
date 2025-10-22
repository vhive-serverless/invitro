package metric

type DeploymentScale struct {
	Timestamp       int64   `csv:"timestamp" json:"timestamp"`
	Function        string  `csv:"function" json:"function"`
	DesiredPods     int     `csv:"desired_pods" json:"desired_pods"`
	RunningPods     int     `csv:"running_pods" json:"running_pods"`
	UnreadyPods     int     `csv:"unready_pods" json:"unready_pods"`
	PendingPods     int     `csv:"pending_pods" json:"pending_pods"`
	TerminatingPods int     `csv:"terminating_pods" json:"terminating_pods"`
	ActivatorQueue  float64 `csv:"activator_queue" json:"activator_queue"`
}

type KnStats struct {
	Timestamp int64 `csv:"timestamp" json:"timestamp"`

	DesiredPods   int `csv:"desired_pods" json:"desired_pods"`
	UnreadyPods   int `csv:"unready_pods" json:"unready_pods"`
	PendingPods   int `csv:"pending_pods" json:"pending_pods"`
	RequestedPods int `csv:"requested_pods" json:"requested_pods"`
	RunningPods   int `csv:"running_pods" json:"running_pods"`

	ActivatorQueue        float64 `csv:"activator_queue" json:"activator_queue"`
	ActivatorRequestCount int     `csv:"activator_request_count" json:"activator_request_count"`
	AutoscalerStableQueue float64 `csv:"autoscaler_stable_queue" json:"autoscaler_stable_queue"`
	AutoscalerPanicQueue  float64 `csv:"autoscaler_panic_queue" json:"autoscaler_panic_queue"`

	SchedulingP95   float64 `csv:"scheduling_p95" json:"scheduling_p95"`
	SchedulingP50   float64 `csv:"scheduling_p50" json:"scheduling_p50"`
	E2ePlacementP95 float64 `csv:"e2e_placement_p95" json:"e2e_placement_p95"`
	E2ePlacementP50 float64 `csv:"e2e_placement_p50" json:"e2e_placement_p50"`
}

type ClusterUsage struct {
	Timestamp       int64     `csv:"timestamp" json:"timestamp"`
	MasterCpuPct    float64   `csv:"master_cpu_pct" json:"master_cpu_pct"`
	MasterCpuReq    float64   `csv:"master_cpu_req" json:"master_cpu_req"`
	MasterCpuLim    float64   `csv:"master_cpu_lim" json:"master_cpu_lim"`
	MasterMemoryPct float64   `csv:"master_mem_pct" json:"master_mem_pct"`
	MasterMemoryReq float64   `csv:"master_mem_req" json:"master_mem_req"`
	MasterMemoryLim float64   `csv:"master_mem_lim" json:"master_mem_lim"`
	MasterPods      int       `csv:"master_pods" json:"master_pods"`
	Cpu             []string  `csv:"cpu" json:"cpu"`
	CpuReq          []float64 `csv:"cpu_req" json:"cpu_req"`
	CpuLim          []float64 `csv:"cpu_lim" json:"cpu_lim"`
	CpuPctAvg       float64   `csv:"cpu_pct_avg" json:"cpu_pct_avg"`
	CpuPctMax       float64   `csv:"cpu_pct_max" json:"cpu_pct_max"`
	CpuPctActiveAvg float64   `csv:"cpu_pct_active_avg" json:"cpu_pct_active_avg"`
	Memory          []string  `csv:"memory" json:"memory"`
	MemoryReq       []float64 `csv:"memory_req" json:"memory_req"`
	MemoryLim       []float64 `csv:"memory_lim" json:"memory_lim"`
	MemoryPctAvg    float64   `csv:"memory_pct" json:"memory_pct"`
	PodCpu          []string  `csv:"pod_cpu" json:"pod_cpu"`
	PodMemory       []string  `csv:"pod_memory" json:"pod_mem"`
	Pods            []int     `csv:"pods" json:"pods"`
	LoaderCpu       float64   `csv:"loader_cpu" json:"loader_cpu"`
	LoaderMem       float64   `csv:"loader_mem" json:"loader_mem"`
}

type AdfResult struct {
	TestStats    float64 `json:"statistic"`
	Pvalue       float64 `json:"pvalue"`
	Lag          int     `json:"usedlag"`
	NumObs       int     `json:"nobs"`
	CriticalVals TValues `json:"critical_vals"`
	IcBest       float64 `json:"icbest"`
}

type TValues struct {
	Pct1  float64 `json:"1%"`
	Pct5  float64 `json:"5%"`
	Pct10 float64 `json:"10%"`
}
