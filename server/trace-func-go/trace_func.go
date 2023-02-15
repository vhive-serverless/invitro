package main

import (
	"flag"
	"github.com/eth-easl/loader/pkg/workload/standard"
	log "github.com/sirupsen/logrus"
	"os"
	"strconv"
)

var (
	zipkin = flag.String("zipkin", "http://zipkin.zipkin:9411/api/v2/spans", "zipkin url")
)

func main() {
	// For containers - port 80; for Firecracker - 50051.
	var serverPort = 80
	var functionType standard.FunctionType

	if len(os.Args) > 1 {
		serverPort, _ = strconv.Atoi(os.Args[1])

		switch os.Args[2] {
		case "EMPTY":
			functionType = standard.EmptyFunction
		default:
			functionType = standard.TraceFunction
		}

		log.Infof("Port: %d\n", serverPort)
		log.Infof("Function type: %s\n", os.Args[2])
	}

	standard.StartGRPCServer("", serverPort, functionType, *zipkin)
}
