package function

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"regexp"
	"strconv"

	log "github.com/sirupsen/logrus"

	tc "github.com/eth-easl/loader/pkg/trace"
	"gopkg.in/yaml.v3"
)

var (
	regex = regexp.MustCompile("at URL:\nhttp://([^\n]+)")
)

type YamlType map[string]interface{}

func readStemYAML(path string, service *YamlType) {
	stem, err := ioutil.ReadFile(path)
	if err != nil {
		log.Fatal("Error reading stem YAML stem.")
	}

	err = yaml.Unmarshal(stem, &service)
	if err != nil {
		log.Fatal("Error unmarshalling stem Knative service.")
	}
}

func addMemoryConstaint(function *tc.Function, knativeService *YamlType) {
	// Reference - https://knative.tips/pod-config/cpu-memory-resources/

	t := ((*knativeService)["spec"].(YamlType))["template"].(YamlType)
	t1 := (t["spec"].(YamlType))["containers"].([]interface{})[0].(YamlType)

	requests := make(map[string]string)
	requests["memory"] = fmt.Sprintf("%dM", function.MemoryStats.Average)
	t1["resources"] = map[string]interface{}{"requests": requests}
}

func writeServiceYaml(outputPath string, knativeService *YamlType) {
	d, err := yaml.Marshal(&knativeService)
	if err != nil {
		log.Fatalf("Error writing modified service file.")
	}

	//fmt.Printf("--- m dump:\n%s\n\n", string(d))
	f, err := os.Create(outputPath)
	if err != nil {
		log.Fatal(err)
	}

	defer f.Close()

	_, err2 := f.WriteString(string(d))
	if err2 != nil {
		log.Fatal(err2)
	}
}

func createCustomYAML(function *tc.Function, parentYAMLPath string) string {
	knativeService := make(YamlType)
	outputFileName := fmt.Sprintf("tmp/%s.yaml", function.Name)

	readStemYAML(parentYAMLPath, &knativeService)
	addMemoryConstaint(function, &knativeService)
	writeServiceYaml(outputFileName, &knativeService)

	return outputFileName
}

func DeployFunctions(functions []tc.Function, serviceConfigPath string, initScales []int) []tc.Function {
	var urls []string
	// deploymentConcurrency := 1 //* Serialise deployment.
	deploymentConcurrency := len(functions) //* Fully parallelise deployment.
	sem := make(chan bool, deploymentConcurrency)

	for funcIdx, function := range functions {
		sem <- true
		go func(function tc.Function, funcIdx int) {
			defer func() { <-sem }()

			var initScale int
			if len(initScales) == len(functions) {
				initScale = initScales[funcIdx]
			} else {
				initScale = 0 //* No-warmup (`initScales` is not populated).
			}
			// log.Info(function.GetName(), " -> initScale: ", initScale)

			customYAMLPath := createCustomYAML(&function, serviceConfigPath)

			hasDeployed := deploy(&function, customYAMLPath, initScale)
			function.SetStatus(hasDeployed)

			if hasDeployed {
				urls = append(urls, function.GetUrl())
			}
			functions[funcIdx] = function // Update function data.
		}(function, funcIdx)
	}
	//* Block until all slots are refilled (after they have all been consumed).
	for i := 0; i < cap(sem); i++ {
		sem <- true
	}

	// defer CreateGrpcPool(functions)
	return functions
}

func deploy(function *tc.Function, serviceConfigPath string, initScale int) bool {
	cmd := exec.Command(
		"kn",
		"service",
		"apply",
		function.Name,
		"-f",
		serviceConfigPath,
		//! `--scale-min` should NOT be used here since it only has a per-revision key,
		//! i.e., it will change the min scale for all the (future) pods of this kn service.
		"--scale-init",
		strconv.Itoa(initScale),
		"--concurrency-target",
		"1",
		//* Wait for infinitely long for ensuring warmup.
		"--wait-timeout",
		"2000000",
	)
	stdoutStderr, err := cmd.CombinedOutput()
	log.Debug("CMD response: ", string(stdoutStderr))

	if err != nil {
		log.Warnf("Failed to deploy function %s: %v\n%s\n", function.Name, err, stdoutStderr)
		return false
	}

	if endpoint := regex.FindStringSubmatch(string(stdoutStderr))[1]; endpoint != function.Endpoint {
		log.Warnf("Update function endpoint to %s\n", endpoint)
		function.Endpoint = endpoint
	}

	log.Info("Deployed function ", function.Endpoint)
	return true
}
