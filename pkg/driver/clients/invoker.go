package clients

import (
	"github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/pkg/common"
	"github.com/vhive-serverless/loader/pkg/config"
	"github.com/vhive-serverless/loader/pkg/metric"
	"sync"
)

type Invoker interface {
	Invoke(*common.Function, *common.RuntimeSpecification) (bool, *metric.ExecutionRecord)
}

func CreateInvoker(cfg *config.LoaderConfiguration, announceDoneExe *sync.WaitGroup, readOpenWhiskMetadata *sync.Mutex) Invoker {
	switch cfg.Platform {
	case "AWSLambda", "AWSLambda-RPS":
		return newAWSLambdaInvoker(announceDoneExe)
	case "Dirigent", "Dirigent-RPS":
		if cfg.InvokeProtocol == "grpc" {
			return newGRPCInvoker(cfg)
		} else {
			return newHTTPInvoker(cfg)
		}
	case "Dirigent-Dandelion", "Dirigent-Dandelion-RPS":
		return newHTTPInvoker(cfg)
	case "Knative", "Knative-RPS":
		if cfg.InvokeProtocol == "grpc" {
			if !cfg.VSwarm {
				return newGRPCInvoker(cfg)
			} else {
				return newGRPCVSwarmInvoker(cfg)
			}
		} else {
			return newHTTPInvoker(cfg)
		}
	case "OpenWhisk", "OpenWhisk-RPS":
		return newOpenWhiskInvoker(announceDoneExe, readOpenWhiskMetadata)
	default:
		logrus.Fatal("Unsupported platform.")
	}

	return nil
}
