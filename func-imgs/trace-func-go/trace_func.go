package main

import (
	"context"
	"fmt"
	"math"
	"net"
	"unsafe"

	"strconv"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	rpc "github.com/eth-easl/easyloader/pkg/faas"
)

var (
	megaByteInByte = uint32(math.Pow(2, 20))
	serverPort     = 80
	pi             = strings.Replace(fmt.Sprintf("%f", math.Pi), ".", "", -1)
)

type funcServer struct {
	//? Embed the following to have forward compatible implementations.
	rpc.UnimplementedExecutorServer
}

func (s *funcServer) Execute(ctx context.Context, req *rpc.FaasRequest) (*rpc.FaasReply, error) {
	start := time.Now()
	runtimeRequested := req.Runtime
	timeoutCh := time.After(time.Duration(runtimeRequested) * time.Millisecond)

	memoryRequested := req.Memory * megaByteInByte   // To bytes.
	buffer := make([]byte, memoryRequested)          // Use `make()` to allocate on heap.
	memoryRequested -= uint32(unsafe.Sizeof(buffer)) // Deduct the memory allocated for the slice reference.

	next := 0
pi_loop:
	for {
		select {
		case <-timeoutCh:
			break pi_loop
		default:
			/** Compute the next digit of π. */
			buffer[next] = pi[next%len(pi)]
		}
		next = int(uint32(next+1) % memoryRequested)
	}

	//TODO: Add the memory allocated to proto.
	memoryAllocated := memoryRequested / megaByteInByte
	return &rpc.FaasReply{
		Response: strconv.Itoa(int(memoryAllocated)),
		Latency:  time.Since(start).Microseconds(),
	}, nil
}

func main() {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", serverPort))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	funcServer := &funcServer{}

	/** The grpcServer is currently configured to serve h2c traffic by default.
	 * To configure credentials or encryption,
	 * see: https://grpc.io/docs/guides/auth.html#go */
	grpcServer := grpc.NewServer()
	/** gRPC Server Reflection provides information about publicly-accessible gRPC services on a server,
	 * and assists clients at runtime to construct RPC requests and responses without precompiled service information.
	 * It is used by gRPC CLI, which can be used to introspect server protos and send/receive test RPCs. */
	reflection.Register(grpcServer)
	rpc.RegisterExecutorServer(grpcServer, funcServer)
	grpcServer.Serve(lis)
}
