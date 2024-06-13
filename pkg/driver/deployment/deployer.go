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
		return &awsLambdaDeployer{}
	case "Dirigent", "Dirigent-RPS", "Dirigent-Dandelion", "Dirigent-Dandelion-RPS":
		return &dirigentDeployer{}
	case "Knative", "Knative-RPS":
		return &knativeDeployer{}
	case "OpenWhisk", "OpenWhisk-RPS":
		return &openWhiskDeployer{}
	default:
		logrus.Fatal("Unsupported platform.")
	}

	return nil
}
