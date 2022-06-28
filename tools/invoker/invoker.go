package main

import (
	"context"
	"flag"
	"time"

	rpc "github.com/eth-easl/loader/server"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

var (
	name    = flag.String("name", "", "Function name")
	runtime = flag.Int("runtime", 1000, "Function runtime target")
	memory  = flag.Int("memory", 170, "Function memory target")
)

func main() {
	flag.Parse()

	if *name == "" {
		log.Fatal("Please specify the function name to invoke.")
	}

	log.Infof("(Invoke)\t %s: %d[µs], %d[MiB]", *name, (*runtime)*1000, *memory)

	// Start latency measurement.
	start := time.Now()
	conn, err := grpc.Dial(*name+".default.10.200.3.4.sslip.io:80", grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		//! Failures will also be recorded with 0's.
		log.Warnf("gRPC connection failed: %v", err)
	}
	defer conn.Close()

	grpcClient := rpc.NewExecutorClient(conn)
	// Contact the server and print out its response.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	response, err := grpcClient.Execute(ctx, &rpc.FaasRequest{
		Message:           "nothing",
		RuntimeInMilliSec: uint32(*runtime),
		MemoryInMebiBytes: uint32(*memory),
	})
	if err != nil {
		log.Warnf("Error in gRPC execution (%s): %v", *name, err)
	}

	responseTime := time.Since(start).Microseconds()
	memoryUsage := response.MemoryUsageInKb
	runtime := response.DurationInMicroSec

	log.Infof("(Replied)\t %s: %d[µs], %d[KB]", *name, runtime, memoryUsage)
	log.Infof("(E2E Latency) %s: %d[µs]\n", *name, responseTime)

}
