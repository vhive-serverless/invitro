package metric

import (
	"sync"
)

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
	*sync.Mutex

	Phase     int   `csv:"phase"`
	Rps       int   `csv:"rps"`
	Timestamp int64 `csv:"timestamp"`
	// FuncName  string `csv:"func_name"`

	//* All time measurements are in microseconds.
	ResponseTime      int64  `csv:"response_time"` //* End-to-end latency.
	RequestedDuration uint32 `csv:"requested_duration"`
	ActualDuration    uint32 `csv:"actual_duration"`
	//* In KiB
	Memory     uint32  `csv:"memory"`
	Timeout    bool    `csv:"timeout"`
	Failed     bool    `csv:"failed"`
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

	ColdStartCount int `csv:"coldstart_count"`
}

type ScaleRecord struct {
	Timestamp    int64  `csv:"timestamp"`
	Deployment   string `csv:"deployment"`
	DesiredScale int    `csv:"desired_scale"`
	ActualScale  int    `csv:"actual_scale"`
}

type DeploymentScale struct {
	Deployment   string `json:"deployment"`
	DesiredScale int    `json:"desired_scale"`
	ActualScale  int    `json:"actual_scale"`
}

type KnStats struct {
	DesiredPods   int `json:"desired_pods"`
	UnreadyPods   int `json:"unready_pods"`
	PendingPods   int `json:"pending_pods"`
	RequestedPods int `json:"requested_pods"`
	RunningPods   int `json:"running_pods"`

	ActivatorQueue        float64 `json:"activator_queue"`
	ActivatorRequestCount int     `json:"activator_request_count"`
	AutoscalerStableQueue float64 `json:"autoscaler_stable_queue"`
	AutoscalerPanicQueue  float64 `json:"autoscaler_panic_queue"`

	SchedulingP99   float64 `json:"scheduling_p99"`
	SchedulingP50   float64 `json:"scheduling_p50"`
	E2ePlacementP99 float64 `json:"e2e_placement_p99"`
	E2ePlacementP50 float64 `json:"e2e_placement_p50"`
}

type ClusterUsage struct {
	MasterCpuPct    float64  `json:"master_cpu_pct"`
	MasterMemoryPct float64  `json:"master_mem_pct"`
	Cpu             []string `json:"cpu"`
	CpuPctAvg       float64  `json:"cpu_pct_avg"`
	CpuPctMax       float64  `json:"cpu_pct_max"`
	CpuPctActiveAvg float64  `json:"cpu_pct_active_avg"`
	Memory          []string `json:"memory"`
	MemoryPctAvg    float64  `json:"memory_pct"`
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
