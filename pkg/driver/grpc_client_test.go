package driver

import (
	"fmt"
	"github.com/eth-easl/loader/pkg/common"
	"github.com/eth-easl/loader/pkg/workload/standard"
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
	success, record := Invoke(&testFunction, &testRuntimeSpecs, true)

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
	address, port := "localhost", 8080
	testFunction.Endpoint = fmt.Sprintf("%s:%d", address, port)

	go standard.StartGRPCServer(address, port, standard.TraceFunction, "")

	// make sure that the gRPC server is running
	time.Sleep(2 * time.Second)

	success, record := Invoke(&testFunction, &testRuntimeSpecs, false)

	if !success ||
		record.ConnectionTimeout != false ||
		record.FunctionTimeout != false ||
		record.ResponseTime == 0 ||
		record.ActualDuration == 0 ||
		record.ActualMemoryUsage == 0 {

		t.Error("Failed gRPC invocations.")
	}
}
