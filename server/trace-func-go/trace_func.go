package main

import (
	"flag"
	"github.com/eth-easl/loader/pkg/workload/standard"
	"os"
	"strconv"
)

var (
	zipkin = flag.String("zipkin", "http://zipkin.zipkin:9411/api/v2/spans", "zipkin url")
)

func main() {
	serverPort := 80 // For containers.
	// serverPort := 50051 // For firecracker.
	if len(os.Args) > 1 {
		serverPort, _ = strconv.Atoi(os.Args[1])
	}

	standard.StartGRPCServer("", serverPort, *zipkin)
}
