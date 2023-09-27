package driver

import (
	log "github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/pkg/common"
)

func DeployFunctionsAWSLambda(functions []*common.Function) {
	provider := "aws"

	// Create serverless.yml file
	serverless := Serverless{}
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

func CleanAWSLambda() {
	CleanServerless()
}
