package deployment

import (
	"github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/pkg/config"
)

type FunctionDeployer interface {
	Deploy(cfg *config.Configuration)
	Clean()
}

func CreateDeployer(cfg *config.Configuration) FunctionDeployer {
	switch cfg.LoaderConfiguration.Platform {
	case "AWSLambda", "AWSLambda-RPS":
		return newAWSLambdaDeployer()
	case "Dirigent", "Dirigent-RPS", "Dirigent-Dandelion", "Dirigent-Dandelion-RPS":
		return newDirigentDeployer()
	case "Knative", "Knative-RPS":
		return newKnativeDeployer()
	case "OpenWhisk", "OpenWhisk-RPS":
		return newOpenWhiskDeployer()
	default:
		logrus.Fatal("Unsupported platform.")
	}

	return nil
}
