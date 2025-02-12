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
	case "AWSLambda":
		return newAWSLambdaDeployer()
	case "Dirigent-Dandelion-Workflow":
		return newDirigentDeployer(true)
	case "Dirigent", "Dirigent-Dandelion":
		return newDirigentDeployer(false)
	case "Knative":
		return newKnativeDeployer()
	case "OpenWhisk":
		return newOpenWhiskDeployer()
	default:
		logrus.Fatal("Unsupported platform.")
	}

	return nil
}
