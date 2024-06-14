package deployment

import (
	log "github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/pkg/common"
	"github.com/vhive-serverless/loader/pkg/config"
)

type awsLambdaDeployer struct{}

type awsLambdaDeploymentConfiguration struct {
}

func newAWSLambdaDeployer() *awsLambdaDeployer {
	return &awsLambdaDeployer{}
}

func newAWSLambdaDeployerConfiguration(_ *config.Configuration) awsLambdaDeploymentConfiguration {
	return awsLambdaDeploymentConfiguration{}
}

func (*awsLambdaDeployer) Deploy(cfg *config.Configuration) {
	internalAWSDeployment(cfg.Functions)
}

func (*awsLambdaDeployer) Clean() {
	CleanServerless()
}

func internalAWSDeployment(functions []*common.Function) {
	provider := "aws"

	// Create serverless.yml file
	serverless := awsServerless{}
	serverless.CreateHeader(provider)
	serverless.AddPackagePattern("./pkg/server/trace-func-go/aws/**")

	for i := 0; i < len(functions); i++ {
		serverless.AddFunctionConfig(functions[i], provider)
	}

	serverless.CreateServerlessConfigFile()

	// Deploy serverless functions and update the function endpoints
	functionToURLMapping := DeployServerless()

	for i := 0; i < len(functions); i++ {
		functions[i].Endpoint = functionToURLMapping[functions[i].Name]
		log.Debugf("Function %s set to %s", functions[i].Name, functions[i].Endpoint)
	}
}
