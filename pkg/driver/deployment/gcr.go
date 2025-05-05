package deployment

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/pkg/common"
	"github.com/vhive-serverless/loader/pkg/config"
	"os/exec"
	"sync"
)

type gcrDeployer struct {
	region    string
	functions []*common.Function
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
	disclaimerText := "You are using Google Cloud Run with InVitro. " +
		"InVitro authors assume no liability for any charges to the InVitro users by the cloud provider. " +
		"InVitro may not remove all the deployed components on Google Cloud Run. " +
		"You should manually check and delete all the created components after an experiment " +
		"so you don't get unpleasant suprises and unexpected bills."
	logrus.Info(disclaimerText)

	gcr.functions = cfg.Functions
	gcr.region = cfg.GCRConfiguration.Region

	wg := sync.WaitGroup{}
	for _, f := range cfg.Functions {
		wg.Add(1)

		go func() {
			defer wg.Done()

			deploySingle(f, cfg)
		}()
	}

	wg.Wait()

	logrus.Infof("All functions have been successfully deployed on Google Cloud Run.")
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
		// TODO: potentially adapt
		fmt.Sprintf("--cpu=%d", 1), // cpu resources
		// TODO: potentially adapt
		fmt.Sprintf("--memory=1Gi"), // memory resources
		fmt.Sprintf("--min-instances=%d", function.DirigentMetadata.ScalingLowerBound), // minimum scale
		fmt.Sprintf("--max-instances=%d", function.DirigentMetadata.ScalingUpperBound), // maximum scale
		fmt.Sprintf("--region=%s", configuration.GCRConfiguration.Region),              // cloud region
		fmt.Sprintf("--project=%s", configuration.GCRConfiguration.Project),            // project name
	}

	if !configuration.GCRConfiguration.AllowUnauthenticated {
		args = append(args, "--no-allow-unauthenticated")
	} else {
		// NOTE: usage of this feature requires run.services.setIamPolicy privilege on Google Cloud account
		args = append(args, "--allow-unauthenticated")
	}

	if configuration.LoaderConfiguration.InvokeProtocol == "grpc" || configuration.LoaderConfiguration.InvokeProtocol == "http2" {
		//args = append(args, "--use-http2") // use HTTP/2 end-to-end
	}

	err := exec.Command("gcloud", args...).Run()
	if err != nil {
		logrus.Errorf("Failed to deploy function %s to Google Cloud Run - %v", function.Name, err)
		return
	}

	// e.g., warm-function-3253699760163344042-328799819690.us-central1.run.app
	function.Endpoint = fmt.Sprintf(
		"%s-%s.%s.run.app",
		function.Name,
		configuration.GCRConfiguration.EndpointSuffix,
		configuration.GCRConfiguration.Region,
	)

	logrus.Infof("Successfully registed %s with Google Cloud Run.", function.Name)
}

func (gcr *gcrDeployer) Clean() {
	wg := sync.WaitGroup{}
	for _, function := range gcr.functions {
		wg.Add(1)

		go func() {
			defer wg.Done()

			args := []string{
				"run",
				"services",
				"delete",
				function.Name,
				fmt.Sprintf("--region=%s", gcr.region),
				"--quiet", // to disable '-y' prompting
			}

			err := exec.Command("gcloud", args...).Run()
			if err != nil {
				logrus.Errorf("Failed to remove function %s from Google Cloud Run - %v", function.Name, err)
				return
			}

			logrus.Infof("Successfully removed %s from Google Cloud Run.", function.Name)
		}()
	}

	wg.Wait()
}
