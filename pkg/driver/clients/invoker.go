package clients

import (
	"sync"

	"github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/pkg/common"
	"github.com/vhive-serverless/loader/pkg/config"
	"github.com/vhive-serverless/loader/pkg/metric"
)

type Invoker interface {
	Invoke(*common.Function, *common.RuntimeSpecification) (bool, *metric.ExecutionRecord)
}

func CreateInvoker(cfg *config.LoaderConfiguration, announceDoneExe *sync.WaitGroup, readOpenWhiskMetadata *sync.Mutex) Invoker {
	switch cfg.Platform {
	case "AWSLambda":
		return newAWSLambdaInvoker(announceDoneExe)
	case "Dirigent":
		if cfg.InvokeProtocol == "grpc" {
			return newGRPCInvoker(cfg, ExecutorRPC{})
		} else {
			return newHTTPInvoker(cfg)
		}
	case "Dirigent-Dandelion", "Dirigent-Dandelion-Workflow":
		return newHTTPInvoker(cfg)
	case "Knative":
		if cfg.InvokeProtocol == "grpc" {
			if !cfg.VSwarm {
				return newGRPCInvoker(cfg, ExecutorRPC{})
			} else {
				return newGRPCInvoker(cfg, SayHelloRPC{})
			}
		} else {
			return newHTTPInvoker(cfg)
		}
	case "OpenWhisk":
		return newOpenWhiskInvoker(announceDoneExe, readOpenWhiskMetadata)
	default:
		logrus.Fatal("Unsupported platform.")
	}

	return nil
}
