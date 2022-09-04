package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"time"

	log "github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	util "github.com/eth-easl/loader/pkg"
	rpc "github.com/eth-easl/loader/server"
)

type funcServer struct {
	rpc.UnimplementedExecutorServer
}

func (s *funcServer) Execute(ctx context.Context, req *rpc.FaasRequest) (*rpc.FaasReply, error) {
	start := time.Now()
	runtimeRequested := req.RuntimeInMilliSec
	timeoutSem := time.After(time.Duration(runtimeRequested) * time.Millisecond)
	if runtimeRequested <= 0 {
		//* Some of the durations were incorrectly recorded as 0 in the trace.
		return &rpc.FaasReply{}, errors.New("non-positive execution time")
	}

	if os.Getenv("ALLOC_VIRTUAL_MEM") == "true" {
		pageSize := unix.Getpagesize()
		numPagesRequested := util.Mib2b(req.MemoryInMebiBytes) / uint32(pageSize)
		//* Golang internally uses `mmap()`, talking to OS directly.
		pages, err := unix.Mmap(-1, 0, int(numPagesRequested)*int(numPagesRequested),
			unix.PROT_WRITE, unix.MAP_ANON|unix.MAP_PRIVATE)
		if err != nil {
			log.Errorf("Failed to allocate requested memory: %v", err)
			return &rpc.FaasReply{}, err
		}
		err = unix.Munmap(pages) //* Don't even touch the allocated pages -> let them stay virtaul memory.
		util.Check(err)
	} else {
		memoryRequestedBytes := util.Mib2b(req.MemoryInMebiBytes)
		//* `make()` gets a piece of initialised memory. No need to touch it.
		_ = make([]byte, memoryRequestedBytes)
	}

	<-timeoutSem //* Blocking wait.
	return &rpc.FaasReply{
		Message:            "Wimpy func -- DONE", // Unused
		DurationInMicroSec: uint32(time.Since(start).Microseconds()),
		MemoryUsageInKb:    util.Mib2Kib(req.MemoryInMebiBytes),
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
	err = grpcServer.Serve(lis)
	util.Check(err)
}
