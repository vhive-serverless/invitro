package driver

import (
	"github.com/eth-easl/loader/pkg/common"
	"testing"
	"time"
)

var testFunction = common.Function{
	Name: "test-function",
}

var testRuntimeSpecs = common.RuntimeSpecification{
	Runtime: 1000, // ms
	Memory:  128,
}

func TestGRPCClientWithServerUnreachable(t *testing.T) {
	success, record := Invoke(testFunction, &testRuntimeSpecs, true)

	if record.FunctionName != "test-function" ||
		record.RequestedDuration != uint32(testRuntimeSpecs.Runtime*1000) ||
		record.StartTime == 0 ||
		record.ResponseTime == 0 ||
		success != false ||
		record.ConnectionTimeout != true {

		t.Error("Error in testing an unreachable server.")
	}
}

func TestGRPCClientWithServerReachable(t *testing.T) {
	time.Sleep(2 * time.Second)

	Invoke(testFunction, &testRuntimeSpecs, true)
}
