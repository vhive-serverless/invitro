package standard

import (
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
	"time"

	tracing "github.com/ease-lab/vhive/utils/tracing/go"
	util "github.com/eth-easl/loader/pkg/common"
	"github.com/eth-easl/loader/pkg/workload/proto"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

// static double SQRTSD (double x) {
//     double r;
//     __asm__ ("sqrtsd %1, %0" : "=x" (r) : "x" (x));
//     return r;
// }
import "C"

const (
	// ContainerImageSizeMB was chosen as a median of the container physical memory usage.
	// Allocate this much less memory inside the actual function.
	ContainerImageSizeMB = 15
)

const EXEC_UNIT int = 1e2

var hostname string
var iterationsMultiplier int
var serverSideCode FunctionType

type FunctionType int

const (
	TraceFunction FunctionType = 0
	EmptyFunction FunctionType = 1
)

func takeSqrts() C.double {
	var tmp C.double // Circumvent compiler optimizations
	for i := 0; i < EXEC_UNIT; i++ {
		tmp = C.SQRTSD(C.double(10))
	}
	return tmp
}

type funcServer struct {
	proto.UnimplementedExecutorServer
}

func busySpin(runtimeMilli uint32) {
	totalIterations := iterationsMultiplier * int(runtimeMilli)

	for i := 0; i < totalIterations; i++ {
		takeSqrts()
	}
}

func (s *funcServer) Execute(_ context.Context, req *proto.FaasRequest) (*proto.FaasReply, error) {
	var msg string
	start := time.Now()

	if serverSideCode == TraceFunction {
		// Minimum execution time is AWS billing granularity - 1ms,
		// as defined in SpecificationGenerator::generateExecutionSpecs
		timeLeftMilliseconds := req.RuntimeInMilliSec
		/*toAllocate := util.Mib2b(req.MemoryInMebiBytes - ContainerImageSizeMB)
		if toAllocate < 0 {
			toAllocate = 0
		}*/

		// make is equivalent to `calloc` in C. The memory gets allocated
		// and zero is written to every byte, i.e. each page should be touched at least once
		//memory := make([]byte, toAllocate)
		// NOTE: the following statement to make sure the compiler does not treat the allocation as dead code
		//log.Debugf("Allocated memory size: %d\n", len(memory))

		timeConsumedMilliseconds := uint32(time.Since(start).Milliseconds())
		if timeConsumedMilliseconds < timeLeftMilliseconds {
			timeLeftMilliseconds -= timeConsumedMilliseconds
			if timeLeftMilliseconds > 0 {
				busySpin(timeLeftMilliseconds)
			}

			msg = fmt.Sprintf("OK asdf- %s", hostname)
		}
	} else {
		msg = fmt.Sprintf("OK - EMPTY asdf- %s", hostname)
	}

	return &proto.FaasReply{
		Message:            msg,
		DurationInMicroSec: uint32(time.Since(start).Microseconds()),
		MemoryUsageInKb:    req.MemoryInMebiBytes * 1024,
	}, nil
}

func readEnvironmentalVariables() {
	if _, ok := os.LookupEnv("ITERATIONS_MULTIPLIER"); ok {
		iterationsMultiplier, _ = strconv.Atoi(os.Getenv("ITERATIONS_MULTIPLIER"))
	} else {
		// Cloudlab xl170 benchmark @ 1 second function execution time
		iterationsMultiplier = 102
	}

	log.Infof("ITERATIONS_MULTIPLIER = %d\n", iterationsMultiplier)

	var err error
	hostname, err = os.Hostname()
	if err != nil {
		log.Warn("Failed to get HOSTNAME environmental variable.")
		hostname = "Unknown host"
	}
}

func StartGRPCServer(serverAddress string, serverPort int, functionType FunctionType, zipkinUrl string) {
	readEnvironmentalVariables()
	serverSideCode = functionType

	if tracing.IsTracingEnabled() {
		log.Infof("Zipkin URL: %s\n", zipkinUrl)
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
	reflection.Register(grpcServer) // gRPC Server Reflection is used by gRPC CLI
	proto.RegisterExecutorServer(grpcServer, &funcServer{})
	err = grpcServer.Serve(lis)
	util.Check(err)
}
