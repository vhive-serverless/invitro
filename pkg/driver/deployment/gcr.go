package deployment

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/pkg/common"
	"github.com/vhive-serverless/loader/pkg/config"
	"os/exec"
)

type gcrDeployer struct {
}

func newGCRDeployer() *gcrDeployer {
	return &gcrDeployer{}
}

func checkIfGcloudUtilityIsInstalled() bool {
	return false
}

func checkContainerImageFormat() bool {
	return false
}

func (gcr *gcrDeployer) Deploy(cfg *config.Configuration) {
	logrus.Infof("You are using Google Cloud Run with InVitro. " +
		"InVitro authors assume no responsibility for any charges to the InVitro users by the cloud provider. " +
		"InVitro may not be removing all deployed entities and this is a reminder to delete all the created " +
		"entities after conducting an experiment session so you don't get unexpected bills.")

	for _, f := range cfg.Functions {
		deploySingle(f, cfg)
	}
}

func deploySingle(function *common.Function, configuration *config.Configuration) {
	args := []string{
		"run",
		"deploy",
		function.Name, // function name
		fmt.Sprintf("--image=%s", function.DirigentMetadata.Image),                                // image
		fmt.Sprintf("--port=%d", function.DirigentMetadata.Port),                                  // port
		fmt.Sprintf("--service-account=%s", configuration.GCRConfiguration.ServiceAccount),        // service account
		fmt.Sprintf("--concurrency=%d", 1),                                                        // container concurrency
		fmt.Sprintf("--timeout=%d", configuration.LoaderConfiguration.GRPCFunctionTimeoutSeconds), // function timeout
		fmt.Sprintf("--cpu=%d", 1),                                                                // cpu resources
		fmt.Sprintf("--memory=1Gi"),                                                               // memory resources
		fmt.Sprintf("--min-instances=%d", function.DirigentMetadata.ScalingLowerBound),            // minimum scale
		fmt.Sprintf("--max-instances=%d", function.DirigentMetadata.ScalingUpperBound),            // maximum scale
		fmt.Sprintf("--region=%s", configuration.GCRConfiguration.Region),                         // cloud region
		fmt.Sprintf("--project=%s", configuration.GCRConfiguration.Project),                       // project name
	}

	if !configuration.GCRConfiguration.AllowUnauthenticated {
		args = append(args, "--no-allow-unauthenticated")
	}
	if configuration.LoaderConfiguration.InvokeProtocol == "grpc" || configuration.LoaderConfiguration.InvokeProtocol == "http2" {
		args = append(args, "--use-http2") // use HTTP/2 end-to-end
	}

	err := exec.Command("gcloud", args...).Run()
	if err != nil {
		logrus.Errorf("Failed to deploy function %s to Google Cloud Run.", function.Name)
		return
	}

	logrus.Infof("Successfully registed %s with the Google Cloud Run.", function.Name)
}

func (gcr *gcrDeployer) Clean() {

}
