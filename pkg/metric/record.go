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
	FunctionName string `csv:"functionName"`
	InvocationID string `csv:"invocationID"`
	StartTime    int64  `csv:"startTime"`

	// Measurements in microseconds
	RequestedDuration uint32 `csv:"requestedDuration"`
	ResponseTime      int64  `csv:"responseTime"`
	ActualDuration    uint32 `csv:"actualDuration"`
	ActualMemoryUsage uint32 `csv:"actualMemoryUsage"`

	ConnectionTimeout bool `csv:"connectionTimeout"`
	FunctionTimeout   bool `csv:"functionTimeout"`


	// In KiB
	//Memory     uint32  `csv:"memory"`
	//MemoryLoad float64 `csv:"mem_load"`
	//Interval   int64   `csv:"interval"`
}

type ScaleRecord struct {
	Timestamp    int64  `csv:"timestamp"`
	Deployment   string `csv:"deployment"`
	DesiredScale int    `csv:"desired_scale"`
	ActualScale  int    `csv:"actual_scale"`
}

type DeploymentScale struct {
	Timestamp    int64  `csv:"timestamp"`
	Deployment   string `json:"deployment"`
	DesiredScale int    `json:"desired_scale"`
	ActualScale  int    `json:"actual_scale"`
}

type KnStats struct {
	Timestamp int64  `csv:"timestamp"`

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
}

type ClusterUsage struct {
	Timestamp    	int64  	 `csv:"timestamp"`
	MasterCpuPct    float64  `csv:"master_cpu_pct"`
	MasterMemoryPct float64  `csv:"master_mem_pct"`
	Cpu             []string `csv:"cpu"`
	CpuPctAvg       float64  `csv:"cpu_pct_avg"`
	CpuPctMax       float64  `csv:"cpu_pct_max"`
	CpuPctActiveAvg float64  `csv:"cpu_pct_active_avg"`
	Memory          []string `csv:"memory"`
	MemoryPctAvg    float64  `csv:"memory_pct"`
}

// TODO: Check if the structs below are necessary
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
