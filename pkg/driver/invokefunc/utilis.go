package invokefunc

import (
	"strings"

	"github.com/eth-easl/loader/pkg/common"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

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