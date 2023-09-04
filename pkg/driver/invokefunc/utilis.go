package invokefunc

import (
	"strings"

	"github.com/eth-easl/loader/pkg/common"
	mc "github.com/eth-easl/loader/pkg/metric"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	return a + b - max(a, b)
}

func sum(numbers []float32) float32 {
	result := float32(0.0)
	for _, num := range numbers {
		result += num
	}
	return result
}

func extractInstanceName(data string) string {
	indexOfHyphen := strings.LastIndex(data, common.FunctionNamePrefix)
	if indexOfHyphen == -1 {
		return data
	}

	return data[indexOfHyphen:]
}

func gRPCConnectionClose(conn *grpc.ClientConn) {
	if conn == nil {
		return
	}

	if err := conn.Close(); err != nil {
		log.Warnf("Error while closing gRPC connection - %s\n", err)
	}
}

func findIndex(list []int, element int) int {
	for i, v := range list {
		if v == element {
			return i
		}
	}
	return -1
}

func prepareMessages(message string, repeat int) string {
	send_messages := ""
	for i := 0; i < repeat; i++ {
		for bsz := 0; bsz < common.BszPerDevice; bsz++ {
			send_messages = send_messages + "@@" + message
		}
	}
	return send_messages
}

func registerJobRecord(jobRecord *mc.JobExecutionRecord,
	startTime, computeTime, executionTime int64,
	replica, gpuCount int,
	startIteration, endIteration, totalIteration int,
	batchSize int) {
	jobRecord.StartTime = append(jobRecord.StartTime, startTime)
	jobRecord.ComputeTime = append(jobRecord.ComputeTime, computeTime)
	jobRecord.ExecutionTime = append(jobRecord.ExecutionTime, executionTime)
	jobRecord.Replica = append(jobRecord.Replica, replica)
	jobRecord.GpuCount = append(jobRecord.GpuCount, gpuCount)
	jobRecord.StartIteration = append(jobRecord.StartIteration, startIteration)
	jobRecord.EndIteration = append(jobRecord.EndIteration, endIteration)
	jobRecord.TotalIteration = append(jobRecord.TotalIteration, totalIteration)
	jobRecord.BatchSize = append(jobRecord.BatchSize, batchSize)

}
