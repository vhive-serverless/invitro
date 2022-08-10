package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	tracing "github.com/ease-lab/vSwarm/utils/tracing/go"
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

func takeSqrts() C.double {
	var tmp C.double //* Circumvent compiler optimisations.
	for i := 0; i < EXEC_UNIT; i++ {
		tmp = C.SQRTSD(C.double(10))
	}
	return tmp
}

type funcServer struct {
	rpc.UnimplementedExecutorServer
}

func busySpin(runtimeMilli uint32) {
	var unitIterations int
	if runtimeMilli > 1e3 {
		unitIterations, _ = strconv.Atoi(os.Getenv("WARM_ITER_PER_1MS"))
	} else {
		unitIterations, _ = strconv.Atoi(os.Getenv("COLD_ITER_PER_1MS"))
	}
	totalIterations := unitIterations * int(runtimeMilli)

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

	//* Offset the time spent on allocating memory.
	msg := "Trace func -- OK"
	if uint32(time.Since(start).Milliseconds()) >= runtimeRequestedMilli {
		msg = "Trace func -- timeout in memory allocation"
	} else {
		runtimeRequestedMilli -= uint32(time.Since(start).Milliseconds())
		busySpin(runtimeRequestedMilli)
	}

	return &rpc.FaasReply{
		Message:            msg,
		DurationInMicroSec: uint32(time.Since(start).Microseconds()),
		MemoryUsageInKb:    util.B2Kib(memoryRequestedBytes),
	}, nil
}

var (
	zipkin = flag.String("zipkin", "http://zipkin.zipkin:9411/api/v2/spans", "zipkin url")
)

func main() {
	serverPort := 80 // For containers.
	// serverPort := 50051 // For firecracker.
	if len(os.Args) > 1 {
		serverPort, _ = strconv.Atoi(os.Args[1])
	}

	if tracing.IsTracingEnabled() {
		log.Printf("Start tracing on : %s\n", *zipkin)
		shutdown, err := tracing.InitBasicTracer(*zipkin, "")
		if err != nil {
			log.Warn(err)
		}
		defer shutdown()
	}

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", serverPort))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	funcServer := &funcServer{}
	var grpcServer *grpc.Server
	if tracing.IsTracingEnabled() {
		grpcServer = tracing.GetGRPCServerWithUnaryInterceptor()
	} else {
		grpcServer = grpc.NewServer()
	}
	reflection.Register(grpcServer) // gRPC Server Reflection is used by gRPC CLI.
	rpc.RegisterExecutorServer(grpcServer, funcServer)
	grpcServer.Serve(lis)
}
