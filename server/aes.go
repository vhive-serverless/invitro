package aes

import (
	"github.com/vhive-serverless/loader/pkg/workload/proto"
	"context"
	"math/rand"
	"crypto/aes"
	"crypto/cipher"
	"encoding/hex"
	"flag"
	"fmt"
	"net"
	"os"
	"strings"
	"time"
	"strconv"
	"github.com/vhive-serverless/loader/pkg/workload/standard"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

var (
	zipkin                    = flag.String("zipkin", "http://localhost:9411/api/v2/spans", "zipkin url")
	address                   = flag.String("addr", "0.0.0.0:50051", "Address:Port the grpc server is listening to")
	key_string                = flag.String("key", "6368616e676520746869732070617373", "The key which is used for encryption")
	default_plaintext_message = flag.String("default-plaintext", "defaultplaintext", "Default plaintext when the function is called with the plaintext_message world")
)
type funcServer struct {
	proto.UnimplementedExecutorServer
}

//Input iterator
func Next() string {
	pkt := "A unique message"
	return pkt
}
func AESModeCTR(plaintext []byte) []byte {
	// Reference: cipher documentation
	// https://golang.org/pkg/crypto/cipher/#Stream

	key, _ := hex.DecodeString(*key_string)
	block, err := aes.NewCipher(key)
	if err != nil {
		panic(err)
	}

	// The IV needs to be unique, but not secure. Therefore it's common to
	// include it at the beginning of the ciphertext.
	// We will use 0 to be predictable
	iv := make([]byte, aes.BlockSize)
	ciphertext := make([]byte, len(plaintext))

	stream := cipher.NewCTR(block, iv)
	stream.XORKeyStream(ciphertext, plaintext)
	return ciphertext
}

func ShowEncryption(in string, timeLeftMilliseconds uint32, start time.Time) resp string {
	var plaintext, ciphertext []byte
	timeConsumedMilliseconds := uint32(time.Since(start).Milliseconds())
	if timeConsumedMilliseconds < timeLeftMilliseconds {
		timeLeftMilliseconds -= timeConsumedMilliseconds
		if timeLeftMilliseconds > 0 {
			if in == "" || in == "world" {
				plaintext = []byte(*default_plaintext_message)
			} else {
				plaintext = []byte(in)
			}
			// Do the encryption
			ciphertext = AESModeCTR(plaintext)
			resp = fmt.Sprintf("fn: AES | plaintext: %s | ciphertext: %x | runtime: golang", plaintext, ciphertext)
			return resp
		}
	}

	return resp
	
	 
}

func (f *funcServer) Execute(_ context.Context, req *proto.FaasRequest) (*proto.FaasReply, error) {
	start := time.Now()
	//generate input
	input := Next()
	//execute benchmark
	timeReq = req.RuntimeInMilliSec
	cipher := ShowEncryption(input, timeReq, start)
	
	return &proto.FaasReply{
		Message: cipher,
		DurationInMicroSec: uint32(time.Since(start).Microseconds()),
		MemoryUsageInKb:    req.MemoryInMebiBytes * 1024,
	}, nil
}

func StartRelayGRPCServer(serverAddress string, serverPort int) {
	lis, err := net.Listen("tcp", fmt.Sprintf("%s:%d", serverAddress, serverPort))
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}
	var grpcServer *grpc.Server
	grpcServer = grpc.NewServer()
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGTERM)

	go func() {
		<-sigc
		log.Info("Gracefully shutting down")
		grpcServer.GracefulStop()
	}()

	reflection.Register(grpcServer)
	proto.RegisterExecutorServer(grpcServer, &funcServer{})
	err = grpcServer.Serve(lis)
}
func main() {
	var serverPort = 80
	ok := os.LookupEnv("FUNC_PORT_ENV"); ok {
		serverPort, _ = strconv.Atoi(os.Getenv("FUNC_PORT_ENV"))
	}
	StartRelayGRPCServer("", serverPort)
}
