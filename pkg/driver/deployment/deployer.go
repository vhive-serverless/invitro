package deployment

import (
	"github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/pkg/config"
)

type FunctionDeployer interface {
	Deploy(cfg *config.Configuration)
	Clean()
}

func CreateDeployer(cfg *config.Configuration) (deployer FunctionDeployer, configuration interface{}) {
	switch cfg.LoaderConfiguration.Platform {
	case "Knative", "Knative-RPS":
		deployer = &knativeDeployer{}
		configuration = knativeDeploymentConfiguration{
			YamlPath:          cfg.YAMLPath,
			IsPartiallyPanic:  cfg.LoaderConfiguration.IsPartiallyPanic,
			EndpointPort:      cfg.LoaderConfiguration.EndpointPort,
			AutoscalingMetric: cfg.LoaderConfiguration.AutoscalingMetric,
		}
	case "OpenWhisk", "OpenWhisk-RPS":
		deployer = &openWhiskDeployer{}
		configuration = openWhiskDeploymentConfiguration{}
	case "AWSLambda", "AWSLambda-RPS":
		deployer = &awsLambdaDeployer{}
		configuration = awsLambdaDeploymentConfiguration{}
	case "Dirigent", "Dirigent-RPS", "Dirigent-Dandelion", "Dirigent-Dandelion-RPS":
		deployer = &dirigentDeployer{}
		configuration = dirigentDeploymentConfiguration{
			RegistrationServer: cfg.LoaderConfiguration.DirigentControlPlaneIP,
		}
	default:
		logrus.Fatal("Unsupported platform.")
	}

	return
}
