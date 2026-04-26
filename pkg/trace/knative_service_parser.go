package trace

import (
	"github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/pkg/common"
	"gopkg.in/yaml.v3"
	"os"
	"strconv"
)

func readKnativeYaml(path string) map[string]any {
	cfg := make(map[string]any)

	yamlFile, err := os.ReadFile(path)
	if err != nil {
		logrus.Fatalf("Error reading Knative YAML - %v", err)
	}

	err = yaml.Unmarshal(yamlFile, &cfg)
	if err != nil {
		logrus.Fatalf("Error unmarshalling Knative YAML - %v", err)
	}

	return cfg
}

func getNodeByName(data []any, key string) map[string]any {
	for i := range data {
		d := data[i].(map[string]any)

		if d["name"] == key {
			return d
		}
	}

	return nil
}

func convertKnativeYamlToDirigentMetadata(path string) *common.DirigentMetadata {
	cfg := readKnativeYaml(path)

	r1 := cfg["spec"].(map[string]any)
	r2 := r1["template"].(map[string]any)

	metadata := r2["metadata"].(map[string]any)
	annotations := metadata["annotations"].(map[string]any)
	upperScale := annotations["autoscaling.knative.dev/max-scale"].(string)
	lowerScale := annotations["autoscaling.knative.dev/min-scale"].(string)
	upperScaleInt, _ := strconv.Atoi(upperScale)
	lowerScaleInt, _ := strconv.Atoi(lowerScale)

	spec := r2["spec"].(map[string]any)
	containers := spec["containers"].([]any)[0].(map[string]any)
	image := containers["image"].(string)

	ports := containers["ports"].([]any)
	port := getNodeByName(ports, "h2c")
	portInt := port["containerPort"].(int)

	env := containers["env"].([]any)
	iterationMultiplier := getNodeByName(env, "ITERATIONS_MULTIPLIER")
	iterationMultiplierInt, _ := strconv.Atoi(iterationMultiplier["value"].(string))

	ioPercentage := getNodeByName(env, "IO_PERCENTAGE")
	ioPercentageInt, _ := strconv.Atoi(ioPercentage["value"].(string))

	return &common.DirigentMetadata{
		Image:               image,
		Port:                portInt,
		Protocol:            "tcp",
		ScalingUpperBound:   upperScaleInt,
		ScalingLowerBound:   lowerScaleInt,
		IterationMultiplier: iterationMultiplierInt,
		IOPercentage:        ioPercentageInt,
	}
}
