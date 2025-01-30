package vswarm

import (
	"context"
	"fmt"
	"net"

	log "github.com/sirupsen/logrus"
	helloworld "github.com/vhive-serverless/vSwarm/utils/protobuf/helloworld"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

type vSwarmServer struct {
	helloworld.UnimplementedGreeterServer
}

func (s *vSwarmServer) SayHello(_ context.Context, req *helloworld.HelloRequest) (*helloworld.HelloReply, error) {
	return &helloworld.HelloReply{
		Message: "Reply message",
	}, nil
}

func StartVSwarmGRPCServer(serverAddress string, serverPort int) {
	lis, err := net.Listen("tcp", fmt.Sprintf("%s:%d", serverAddress, serverPort))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()

	reflection.Register(grpcServer) // gRPC Server Reflection is used by gRPC CLI
	helloworld.RegisterGreeterServer(grpcServer, &vSwarmServer{})
	_ = grpcServer.Serve(lis)
}
