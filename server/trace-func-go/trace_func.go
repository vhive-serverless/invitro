package main

import (
	"context"
	"errors"
	"fmt"
	"net"

	"time"

	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	rpc "github.com/eth-easl/loader/server"
)

var (
	start time.Time
	//* Use `make()` to allocate a static array (NOT a linked slice) on heap.
	osPage     = [4 * 1024]byte{}
	kb         = cap(osPage) / 4
	serverPort = 80
)

type funcServer struct {
	rpc.UnimplementedExecutorServer
}

func (s *funcServer) Execute(ctx context.Context, req *rpc.FaasRequest) (*rpc.FaasReply, error) {
	start = time.Now()

	runtimeRequested := req.RuntimeInMilliSec
	memoryRequestedInKbs := req.MemoryInMebiBytes * uint32(kb) // MiB to KB.
	timeoutSem := time.After(time.Duration(runtimeRequested) * time.Millisecond)

	//* To avoid unecessary overhead, memory allocation is at the granularity of linux pages.
	numPagesRequested := memoryRequestedInKbs/4 - 1
	for i := 0; i < int(numPagesRequested); i++ {
		select {
		case <-timeoutSem:
			return makeReply(uint32((i + 1) * 4 * kb)),
				errors.New("timeout when allocating memory")
		default:
			//* Create a NEW page by copying the existing one.
			newPage := osPage
			//* Write to the first byte to consolidate virtual mem. into physical mem.
			newPage[0] = byte(i)
		}
	}

	<-timeoutSem //* Fulfil requested runtime.
	return makeReply(memoryRequestedInKbs), nil
}

func makeReply(memoryUsage uint32) *rpc.FaasReply {
	return &rpc.FaasReply{
		Message:           "", // Unused
		LatencyInMicroSec: uint32(time.Since(start).Microseconds()),
		MemoryUsageInKb:   memoryUsage,
	}
}

func main() {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", serverPort))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	funcServer := &funcServer{}
	grpcServer := grpc.NewServer()
	reflection.Register(grpcServer) // gRPC Server Reflection is used by gRPC CLI.
	rpc.RegisterExecutorServer(grpcServer, funcServer)
	grpcServer.Serve(lis)
}
