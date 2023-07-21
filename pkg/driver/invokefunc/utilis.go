package invokefunc

import (
	"strings"

	"github.com/eth-easl/loader/pkg/common"
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
