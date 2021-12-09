package function

import (
	"fmt"
	"os/exec"
	"path/filepath"

	log "github.com/sirupsen/logrus"

	fc "github.com/eth-easl/easyloader/internal/function"
	// tc "github.com/eth-easl/easyloader/internal/trace"
)

// Functions is an object for unmarshalled JSON with functions to deploy.
type Functions struct {
	Functions []FunctionType `json:"functions"`
}

type FunctionType struct {
	Name string `json:"name"`
	File string `json:"file"`

	// Number of functions to deploy from the same file (with different names)
	Count int `json:"count"`

	Eventing    bool   `json:"eventing"`
	ApplyScript string `json:"applyScript"`
}

const (
	gatewayURL    = "192.168.1.240.sslip.io"
	namespaceName = "default"
)

func Deploy(functions []fc.Function, workloadPath string, deploymentConcurrency int) []string {
	var urls []string
	/**
	 * Limit the number of parallel deployments
	 * using a channel (semaphore).
	 */
	sem := make(chan bool, deploymentConcurrency)

	// log.Info("funcSlice: ", funcSlice)
	for _, function := range functions {
		for i := 0; i < fType.Count; i++ {

			sem <- true

			funcName := fmt.Sprintf("%s-%d", fType.Name, i)
			url := fmt.Sprintf("%s.%s.%s", funcName, namespaceName, gatewayURL)
			urls = append(urls, url)

			filePath := filepath.Join(funcPath, fType.File)

			go func(funcName, filePath string) {
				defer func() { <-sem }()

				deployFunction(funcName, filePath)
			}(funcName, filePath)
		}
	}

	for i := 0; i < cap(sem); i++ {
		sem <- true
	}

	return urls
}

func deployFunction(funcName, filePath string) {
	cmd := exec.Command(
		"kn",
		"service",
		"apply",
		funcName,
		"-f",
		filePath,
		"--concurrency-target",
		"1",
	)
	stdoutStderr, err := cmd.CombinedOutput()
	log.Debug("CMD response: ", string(stdoutStderr))

	if err != nil {
		log.Warnf("Failed to deploy function %s, %s: %v\n%s\n", funcName, filePath, err, stdoutStderr)
	}

	log.Info("Deployed function ", funcName)
}
