/*
 * MIT License
 *
 * Copyright (c) 2023 EASL and the vHive community
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 */

package metric

type StartType string

const (
	Hot  StartType = "hot"
	Cold StartType = "cold"
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

type ExecutionRecordBase struct {
	Phase        int    `csv:"phase"`
	Instance     string `csv:"instance"`
	InvocationID string `csv:"invocationID"`
	Function     string `csv:"functionName"`
	StartTime    int64  `csv:"startTime"`

	// Measurements in microseconds
	RequestedDuration           uint32 `csv:"requestedDuration"`
	GRPCConnectionEstablishTime int64  `csv:"grpcConnEstablish"`
	ResponseTime                int64  `csv:"responseTime"`
	ActualDuration              uint32 `csv:"actualDuration"`

	ConnectionTimeout bool `csv:"connectionTimeout"`
	FunctionTimeout   bool `csv:"functionTimeout"`
}

type ExecutionRecordOpenWhisk struct {
	ExecutionRecordBase

	ActivationID   string    `csv:"activationID"`
	StartType      StartType `csv:"startType"`
	HttpStatusCode int       `csv:"httpStatusCode"`

	// Measurements in microseconds
	WaitTime int64 `csv:"waitTime"`
	InitTime int64 `csv:"initTime"`
}

type ExecutionRecord struct {
	ExecutionRecordBase

	// Measurements in microseconds
	ActualMemoryUsage       uint32 `csv:"actualMemoryUsage"`
	MemoryAllocationTimeout bool   `csv:"memoryAllocationTimeout"`

	AsyncResponseID     string `csv:"-"`
	TimeToSubmitMs      int64  `csv:"timeToSubmitMs"`
	UserCodeExecutionMs int64  `csv:"userCodeExecutionMs"`

	TimeToGetResponseMs int64 `csv:"timeToGetResponseMs"`
}

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
