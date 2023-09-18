/*
 * MIT License
 *
 * Copyright (c) 2023 EASL and the vHive community
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 */

package main

import (
	"context"
	"errors"
	"fmt"
	util "github.com/vhive-serverless/loader/pkg/common"
	"github.com/vhive-serverless/loader/pkg/workload/proto"
	"net"
	"time"

	log "github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

type funcServer struct {
	proto.UnimplementedExecutorServer
}

func (s *funcServer) Execute(ctx context.Context, req *proto.FaasRequest) (*proto.FaasReply, error) {
	start := time.Now()
	runtimeRequested := req.RuntimeInMilliSec
	timeoutSem := time.After(time.Duration(runtimeRequested) * time.Millisecond)
	if runtimeRequested <= 0 {
		//* Some of the durations were incorrectly recorded as 0 in the trace.
		return &proto.FaasReply{}, errors.New("non-positive execution time")
	}

	pageSize := unix.Getpagesize()
	numPagesRequested := util.Mib2b(req.MemoryInMebiBytes) / uint32(pageSize)
	//* Golang internally uses `mmap()`, talking to OS directly.
	pages, err := unix.Mmap(-1, 0, int(numPagesRequested)*int(numPagesRequested),
		unix.PROT_WRITE, unix.MAP_ANON|unix.MAP_PRIVATE)
	if err != nil {
		log.Errorf("Failed to allocate requested memory: %v", err)
		return &proto.FaasReply{}, err
	}
	err = unix.Munmap(pages) //* Don't even touch the allocated pages -> let them stay virtaul memory.
	util.Check(err)

	<-timeoutSem //* Blocking wait.
	return &proto.FaasReply{
		Message:            "Wimpy func -- DONE", // Unused
		DurationInMicroSec: uint32(time.Since(start).Microseconds()),
		MemoryUsageInKb:    util.B2Kib(numPagesRequested * uint32(pageSize)),
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
	proto.RegisterExecutorServer(grpcServer, funcServer)
	err = grpcServer.Serve(lis)
	util.Check(err)
}
