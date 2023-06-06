package main

import (
	"context"
	"log"

	pb "github.com/eth-easl/loader/pkg/workload/proto"

	"google.golang.org/grpc"
)

func main() {
	// create a connection to the gRPC server
	conn, err := grpc.Dial("gpt.default.10.200.3.4.sslip.io:80", grpc.WithInsecure())
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	// create a client stub for the gRPC service
	client := pb.NewExecutorClient(conn)

	// create a request message
	request := &pb.FaasRequest{
		Message:              "Hello, world!",
		RuntimeInMilliSec:    10,
		GpuMemoryInMebiBytes: 10,
	}

	// send the request to the gRPC server
	response, err := client.Execute(context.Background(), request)
	if err != nil {
		log.Fatalf("Failed to execute: %v", err)
	}

	// print the response message
	log.Printf("Response: %v", response)
}
