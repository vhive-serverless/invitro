package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"time"

	log "github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	util "github.com/eth-easl/loader/pkg"
	rpc "github.com/eth-easl/loader/server"
)

//! We don't enforce this limit anymore because no limits have set for the containers themselves
//! (i.e., they are busrtable workloads controlled by K8s and won't get OOM-killed by the kernel).
// const containerMemoryLimitMib = 512 // Default limit of k8s.

type funcServer struct {
	rpc.UnimplementedExecutorServer
}

func busySpin(timeoutSem <-chan time.Time) {
	/** `for { }` generates the assembly `jmp self`, which is a spin lock. */
	for {
		select {
		case <-timeoutSem: //* Fulfill requested runtime.
			return
		default: //* Non-blocking.
			continue
		}
	}
}

func (s *funcServer) Execute(ctx context.Context, req *rpc.FaasRequest) (*rpc.FaasReply, error) {
	start := time.Now()
	runtimeRequested := req.RuntimeInMilliSec
	timeoutSem := time.After(time.Duration(runtimeRequested) * time.Millisecond)
	if runtimeRequested < 1 {
		//* Some of the durations were incorrectly recorded as 0 in the trace.
		return &rpc.FaasReply{}, errors.New("erroneous execution time")
	}

	//* To avoid unecessary overheads, memory allocation is at the granularity of os pages.
	numPagesRequested := util.Mib2b(req.MemoryInMebiBytes) / uint32(unix.Getpagesize())
	pageSize := unix.Getpagesize()
	pages := make([]byte, numPagesRequested*uint32(pageSize))

	//* Only touch the first page.
	pages[0] = byte(1)

	var err error = nil
	if uint32(time.Since(start).Milliseconds()) > runtimeRequested {
		err = errors.New("timeout in function excecution")
	}

	busySpin(timeoutSem)

	return &rpc.FaasReply{
		Message:            "", // Unused
		DurationInMicroSec: uint32(time.Since(start).Microseconds()),
		MemoryUsageInKb:    util.B2Kib(numPagesRequested * uint32(unix.Getpagesize())),
	}, err
}

func main() {
	serverPort := 80 // For containers.
	// 50051 for firecracker.
	if len(os.Args) > 1 {
		serverPort, _ = strconv.Atoi(os.Args[1])
	}

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
