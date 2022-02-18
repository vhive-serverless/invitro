package metric

import (
	"sync"
)

type MinuteInvocationRecord struct {
	Phase           int   `csv:"phase"`
	Rps             int   `csv:"request_per_sec"`
	MinuteIdx       int   `csv:"index"`
	Duration        int64 `csv:"duration"`
	NumFuncTargeted int   `csv:"num_func_target"`
	NumFuncInvoked  int   `csv:"num_func_invoked"`
	NumFuncFailed   int   `csv:"num_func_failed"`
}

type ExecutionRecord struct {
	*sync.Mutex

	Phase        int    `csv:"phase"`
	Rps          int    `csv:"request_per_sec"`
	Timestamp    int64  `csv:"timestamp"`
	FuncName     string `csv:"func_name"`
	ResponseTime int64  `csv:"response_time"` //* End-to-end latency.
	Runtime      uint32 `csv:"runtime"`
	Memory       uint32 `csv:"memory"`
	Timeout      bool   `csv:"timeout"`
	Failed       bool   `csv:"failed"`
}
