package standard

import (
	"context"
	"errors"
	"fmt"
	tracing "github.com/ease-lab/vhive/utils/tracing/go"
	util "github.com/eth-easl/loader/pkg/common"
	"github.com/eth-easl/loader/pkg/workload/proto"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"net"
	"os"
	"strconv"
	"time"
)

// static double SQRTSD (double x) {
//     double r;
//     __asm__ ("sqrtsd %1, %0" : "=x" (r) : "x" (x));
//     return r;
// }
import "C"

const EXEC_UNIT int = 1e2

var hostname = "Unknown host"
var warmIteration = 115
var coldIteration = 90
var serversideCode FunctionType

type FunctionType int

const (
	TraceFunction FunctionType = 0
	EmptyFunction FunctionType = 1
)

func takeSqrts() C.double {
	var tmp C.double //* Circumvent compiler optimisations.
	for i := 0; i < EXEC_UNIT; i++ {
		tmp = C.SQRTSD(C.double(10))
	}
	return tmp
}

type funcServer struct {
	proto.UnimplementedExecutorServer
}

func busySpin(runtimeMilli uint32) {
	var unitIterations int
	if runtimeMilli > 1e3 {
		unitIterations = warmIteration
	} else {
		unitIterations = coldIteration
	}
	totalIterations := unitIterations * int(runtimeMilli)

	for i := 0; i < totalIterations; i++ {
		takeSqrts()
	}
}

func (s *funcServer) Execute(_ context.Context, req *proto.FaasRequest) (*proto.FaasReply, error) {
	var msg string
	var memoryRequestedBytes uint32
	start := time.Now()

	if serversideCode == TraceFunction {
		runtimeRequestedMilli := req.RuntimeInMilliSec
		if runtimeRequestedMilli <= 0 {
			// Some durations were incorrectly recorded as 0 in the trace.
			return &proto.FaasReply{}, errors.New("non-positive execution time")
		}

		memoryRequestedBytes = util.Mib2b(req.MemoryInMebiBytes)
		// `make()` gets a piece of initialised memory. No need to touch it.
		_ = make([]byte, memoryRequestedBytes)

		// Offset the time spent on allocating memory.
		msg = fmt.Sprintf("OK - %s", hostname)
		if uint32(time.Since(start).Milliseconds()) >= runtimeRequestedMilli {
			msg = fmt.Sprintf("FAILURE - mem_alloc timeout - %s", hostname)
		} else {
			runtimeRequestedMilli -= uint32(time.Since(start).Milliseconds())
			busySpin(runtimeRequestedMilli)
		}
	} else {
		msg = fmt.Sprintf("EMPTY OK - %s", hostname)
	}

	return &proto.FaasReply{
		Message:            msg,
		DurationInMicroSec: uint32(time.Since(start).Microseconds()),
		MemoryUsageInKb:    util.B2Kib(memoryRequestedBytes),
	}, nil
}

func readEnvironmentalVariables() {
	if _, ok := os.LookupEnv("WARM_ITER_PER_1MS"); ok {
		warmIteration, _ = strconv.Atoi(os.Getenv("WARM_ITER_PER_1MS"))
	} else {
		warmIteration = 115
	}

	if _, ok := os.LookupEnv("COLD_ITER_PER_1MS"); ok {
		coldIteration, _ = strconv.Atoi(os.Getenv("COLD_ITER_PER_1MS"))
	} else {
		coldIteration = 90
	}

	var err error
	hostname, err = os.Hostname()
	if err != nil {
		log.Info("Failed to get HOSTNAME environmental variable.")
	}
}

func StartGRPCServer(serverAddress string, serverPort int, functionType FunctionType, zipkinUrl string) {
	readEnvironmentalVariables()
	serversideCode = functionType

	if tracing.IsTracingEnabled() {
		log.Printf("Start tracing on : %s\n", zipkinUrl)
		shutdown, err := tracing.InitBasicTracer(zipkinUrl, "")
		if err != nil {
			log.Warn(err)
		}
		defer shutdown()
	}

	lis, err := net.Listen("tcp", fmt.Sprintf("%s:%d", serverAddress, serverPort))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	var grpcServer *grpc.Server
	if tracing.IsTracingEnabled() {
		grpcServer = tracing.GetGRPCServerWithUnaryInterceptor()
	} else {
		grpcServer = grpc.NewServer()
	}
	reflection.Register(grpcServer) // gRPC Server Reflection is used by gRPC CLI.
	proto.RegisterExecutorServer(grpcServer, &funcServer{})
	err = grpcServer.Serve(lis)
	util.Check(err)
}
