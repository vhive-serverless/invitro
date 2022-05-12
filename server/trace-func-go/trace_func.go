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
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	util "github.com/eth-easl/loader/pkg"
	rpc "github.com/eth-easl/loader/server"
)

// static double SQRTSD (double x) {
//     double r;
//     __asm__ ("sqrtsd %1, %0" : "=x" (r) : "x" (x));
//     return r;
// }
import "C"

const EXEC_UNIT int = 1e2

func takeSqrts() {
	for i := 0; i < EXEC_UNIT; i++ {
		_ = C.SQRTSD(C.double(10))
	}
}

type funcServer struct {
	rpc.UnimplementedExecutorServer
}

func busySpin(runtimeMilli uint32) {
	delta := 16 // Emperical skewness.
	unitIterations, _ := strconv.Atoi(os.Getenv("AVG_ITER_PER_1MS"))
	totalIterations := (unitIterations - delta) * int(runtimeMilli)

	for i := 0; i < totalIterations; i++ {
		takeSqrts()
	}
}

func (s *funcServer) Execute(ctx context.Context, req *rpc.FaasRequest) (*rpc.FaasReply, error) {
	start := time.Now()
	runtimeRequestedMilli := req.RuntimeInMilliSec
	if runtimeRequestedMilli <= 0 {
		//* Some of the durations were incorrectly recorded as 0 in the trace.
		return &rpc.FaasReply{}, errors.New("non-positive execution time")
	}

	memoryRequestedBytes := util.Mib2b(req.MemoryInMebiBytes)
	//* `make()` gets a piece of initialised memory. No need to touch it.
	_ = make([]byte, memoryRequestedBytes)

	busySpin(runtimeRequestedMilli)

	return &rpc.FaasReply{
		Message:            "Trace func -- OK",
		DurationInMicroSec: uint32(time.Since(start).Microseconds()),
		MemoryUsageInKb:    util.B2Kib(memoryRequestedBytes),
	}, nil
}

func main() {
	serverPort := 80 // For containers.
	// serverPort := 50051 for firecracker.
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
