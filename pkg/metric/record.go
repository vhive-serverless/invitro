package metric

type MinuteInvocationRecord struct {
	Phase           int   `csv:"phase"`
	Rps             int   `csv:"rps"`
	MinuteIdx       int   `csv:"index"`
	Duration        int64 `csv:"duration"`
	NumFuncTargeted int   `csv:"num_func_target"`
	NumFuncInvoked  int   `csv:"num_func_invoked"`
	NumColdStarts   int   `csv:"num_coldstarts"`
}

type ExecutionRecord struct {
	Phase        int    `csv:"phase"`
	Instance     string `csv:"instance"`
	InvocationID string `csv:"invocationID"`
	StartTime    int64  `csv:"startTime"`

	// Measurements in microseconds
	RequestedDuration uint32 `csv:"requestedDuration"`
	ResponseTime      int64  `csv:"responseTime"`
	ActualDuration    uint32 `csv:"actualDuration"`
	ActualMemoryUsage uint32 `csv:"actualMemoryUsage"`

	MemoryAllocationTimeout bool `csv:"memoryAllocationTimeout"`
	ConnectionTimeout       bool `csv:"connectionTimeout"`
	FunctionTimeout         bool `csv:"functionTimeout"`

	// TODO: EVERYTHING BELOW ARE UNTESTED FIELDS

	//* In KiB
	/*Memory     uint32  `csv:"memory"`
	MemoryLoad float64 `csv:"mem_load"`
	Interval   int64   `csv:"interval"`

	//* Infra statistics are all in percentages.
	MasterCpu          float64 `csv:"master_cpu"`
	MasterMem          float64 `csv:"master_mem"`
	WorkerCpuAvg       float64 `csv:"worker_cpu_avg"`
	WorkerCpuMax       float64 `csv:"worker_cpu_max"`
	WorkerCpuActiveAvg float64 `csv:"worker_cpu_active_avg"`
	WorkerMemAvg       float64 `csv:"worker_mem"`

	DesiredPods   int `csv:"desired_pods"`
	UnreadyPods   int `csv:"unready_pods"`
	PendingPods   int `csv:"pending_pods"`
	RequestedPods int `csv:"requested_pods"`
	RunningPods   int `csv:"running_pods"`

	ActivatorQueue        float64 `csv:"activator_queue"`
	ActivatorRequestCount int     `csv:"activator_request_count"`
	AutoscalerStableQueue float64 `csv:"autoscaler_stable_queue"`
	AutoscalerPanicQueue  float64 `csv:"autoscaler_panic_queue"`

	SchedulingP99   float64 `csv:"scheduling_p99"`
	SchedulingP50   float64 `csv:"scheduling_p50"`
	E2ePlacementP99 float64 `csv:"e2e_placement_p99"`
	E2ePlacementP50 float64 `csv:"e2e_placement_p50"`

	ColdStartCount int `csv:"coldstart_count"`*/
}

type ScaleRecord struct {
	Timestamp    int64  `csv:"timestamp"`
	Deployment   string `csv:"deployment"`
	DesiredScale int    `csv:"desired_scale"`
	ActualScale  int    `csv:"actual_scale"`
}

type DeploymentScale struct {
	Timestamp    int64  `csv:"timestamp"`
	Deployment   string `csv:"deployment"`
	DesiredScale int    `csv:"desired_scale"`
	ActualScale  int    `csv:"actual_scale"`
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

	SchedulingP95   float64 `csv:"scheduling_p99" json:"scheduling_p95"`
	SchedulingP50   float64 `csv:"scheduling_p50" json:"scheduling_p50"`
	E2ePlacementP95 float64 `csv:"e2e_placement_p99" json:"e2e_placement_p95"`
	E2ePlacementP50 float64 `csv:"e2e_placement_p50" json:"e2e_placement_p50"`
}

type ClusterUsage struct {
	Timestamp       int64    `csv:"timestamp" json:"timestamp"`
	MasterCpuPct    float64  `csv:"master_cpu_pct" json:"master_cpu_pct"`
	MasterMemoryPct float64  `csv:"master_mem_pct" json:"master_mem_pct"`
	Cpu             []string `csv:"cpu" json:"cpu"`
	CpuPctAvg       float64  `csv:"cpu_pct_avg" json:"cpu_pct_avg"`
	CpuPctMax       float64  `csv:"cpu_pct_max" json:"cpu_pct_max"`
	CpuPctActiveAvg float64  `csv:"cpu_pct_active_avg" json:"cpu_pct_active_avg"`
	Memory          []string `csv:"memory" json:"memory"`
	MemoryPctAvg    float64  `csv:"memory_pct" json:"memory_pct"`
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
