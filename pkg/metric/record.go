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
	NumFuncFailed   int   `csv:"num_func_failed"`
}

type ExecutionRecord struct {
	*sync.Mutex

	Phase        int     `csv:"phase"`
	Rps          int     `csv:"rps"`
	Timestamp    int64   `csv:"timestamp"`
	FuncName     string  `csv:"func_name"`
	ResponseTime int64   `csv:"response_time"` //* End-to-end latency.
	Runtime      uint32  `csv:"runtime"`
	Memory       uint32  `csv:"memory"`
	Timeout      bool    `csv:"timeout"`
	Failed       bool    `csv:"failed"`
	Load         float64 `csv:"load"`
	Interval     int64   `csv:"interval"`

	//* Infra statistics are all in percentages.
	MasterCpu    float64 `csv:"master_cpu"`
	MasterMem    float64 `csv:"master_mem"`
	WorkerCpuAvg float64 `csv:"worker_cpu"`
	WorkerMemAvg float64 `csv:"worker_mem"`

	DesiredPods    int `csv:"desired_pods"`
	UnreadyPods    int `csv:"unready_pods"`
	PendingPods    int `csv:"pending_pods"`
	RequestedPods  int `csv:"requested_pods"`
	RunningPods    int `csv:"running_pods"`
	ColdStartCount int `csv:"coldstart_count"`

	ActivatorQueue        float64 `csv:"activator_queue"`
	AutoscalerStableQueue float64 `csv:"autoscaler_stable_queue"`
	AutoscalerPanicQueue  float64 `csv:"autoscaler_panic_queue"`
}

type KnStats struct {
	DesiredPods    int `json:"desired_pods"`
	UnreadyPods    int `json:"unready_pods"`
	PendingPods    int `json:"pending_pods"`
	RequestedPods  int `json:"requested_pods"`
	RunningPods    int `json:"running_pods"`
	ColdStartCount int `json:"coldstart_count"`

	ActivatorQueue        float64 `json:"activator_queue"`
	AutoscalerStableQueue float64 `json:"autoscaler_stable_queue"`
	AutoscalerPanicQueue  float64 `json:"autoscaler_panic_queue"`
}

type ClusterUsage struct {
	MasterCpuPct    float64  `json:"master_cpu_pct"`
	MasterMemoryPct float64  `json:"master_mem_pct"`
	Cpu             []string `json:"cpu"`
	CpuPctAvg       float64  `json:"cpu_pct"`
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
