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

func CreateInvoker(cfg *config.Configuration, announceDoneExe *sync.WaitGroup, readOpenWhiskMetadata *sync.Mutex) Invoker {
	switch cfg.LoaderConfiguration.Platform {
	case "AWSLambda":
		return newAWSLambdaInvoker(announceDoneExe)
	case "Dirigent":
		if cfg.DirigentConfiguration == nil {
			logrus.Fatal("Failed to create invoker: dirigent configuration is required for platform 'dirigent'")
		}
		if cfg.DirigentConfiguration.Backend == "Dandelion" || cfg.LoaderConfiguration.InvokeProtocol != "grpc" {
			return newHTTPInvoker(cfg)
		} else {
			return newGRPCInvoker(cfg.LoaderConfiguration, ExecutorRPC{})
		}
	case "Knative":
		if cfg.LoaderConfiguration.InvokeProtocol == "grpc" {
			if !cfg.LoaderConfiguration.VSwarm {
				return newGRPCInvoker(cfg.LoaderConfiguration, ExecutorRPC{})
			} else {
				return newGRPCInvoker(cfg.LoaderConfiguration, SayHelloRPC{})
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
