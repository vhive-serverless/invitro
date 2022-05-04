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

type funcServer struct {
	rpc.UnimplementedExecutorServer
}

func (s *funcServer) Execute(ctx context.Context, req *rpc.FaasRequest) (*rpc.FaasReply, error) {
	runtimeRequested := req.RuntimeInMilliSec
	if runtimeRequested < 1 {
		//* Some of the durations were incorrectly recorded as 0 in the trace.
		return &rpc.FaasReply{}, errors.New("erroneous execution time")
	}

	start := time.Now()
	timeoutSem := time.After(time.Duration(runtimeRequested) * time.Millisecond)

	<-timeoutSem //* Fulfill requested runtime.
	return &rpc.FaasReply{
		Message:            "", // Unused
		DurationInMicroSec: uint32(time.Since(start).Microseconds()),
		MemoryUsageInKb:    0,
	}, nil
}

func main() {
	serverPort := 80

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
