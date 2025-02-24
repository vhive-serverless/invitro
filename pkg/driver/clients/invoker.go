package clients

import (
	"strings"
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
	switch strings.ToLower(cfg.LoaderConfiguration.Platform) {
	case common.PlatformAWSLambda:
		return newAWSLambdaInvoker(announceDoneExe)
	case common.PlatformDirigent:
		if cfg.DirigentConfiguration == nil {
			logrus.Fatal("Failed to create invoker: dirigent configuration is required for platform 'dirigent'")
		}
		if strings.ToLower(cfg.DirigentConfiguration.Backend) == common.BackendDandelion || cfg.LoaderConfiguration.InvokeProtocol != "grpc" {
			return newHTTPInvoker(cfg)
		} else {
			return newGRPCInvoker(cfg.LoaderConfiguration, ExecutorRPC{})
		}
	case common.PlatformKnative:
		if cfg.LoaderConfiguration.InvokeProtocol == "grpc" {
			if !cfg.LoaderConfiguration.VSwarm {
				return newGRPCInvoker(cfg.LoaderConfiguration, ExecutorRPC{})
			} else {
				return newGRPCInvoker(cfg.LoaderConfiguration, SayHelloRPC{})
			}
		} else {
			return newHTTPInvoker(cfg)
		}
	case common.PlatformOpenWhisk:
		return newOpenWhiskInvoker(announceDoneExe, readOpenWhiskMetadata)
	default:
		logrus.Fatal("Unsupported platform.")
	}

	return nil
}
