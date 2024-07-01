package trace

import (
	"github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/pkg/common"
	"gopkg.in/yaml.v3"
	"os"
	"strconv"
)

func readKnativeYaml(path string) map[string]interface{} {
	cfg := make(map[string]interface{})

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

func getNodeByName(data []interface{}, key string) map[string]interface{} {
	for i := 0; i < len(data); i++ {
		d := data[i].(map[string]interface{})

		if d["name"] == key {
			return d
		}
	}

	return nil
}

func convertKnativeYamlToDirigentMetadata(path string) *common.DirigentMetadata {
	cfg := readKnativeYaml(path)

	r1 := cfg["spec"].(map[string]interface{})
	r2 := r1["template"].(map[string]interface{})

	metadata := r2["metadata"].(map[string]interface{})
	annotations := metadata["annotations"].(map[string]interface{})
	upperScale := annotations["autoscaling.knative.dev/max-scale"].(string)
	lowerScale := annotations["autoscaling.knative.dev/min-scale"].(string)
	upperScaleInt, _ := strconv.Atoi(upperScale)
	lowerScaleInt, _ := strconv.Atoi(lowerScale)

	spec := r2["spec"].(map[string]interface{})
	containers := spec["containers"].([]interface{})[0].(map[string]interface{})
	image := containers["image"].(string)

	ports := containers["ports"].([]interface{})
	port := getNodeByName(ports, "h2c")
	portInt := port["containerPort"].(int)

	env := containers["env"].([]interface{})
	iterationMultiplier := getNodeByName(env, "ITERATIONS_MULTIPLIER")
	iterationMultiplierInt, _ := strconv.Atoi(iterationMultiplier["value"].(string))

	return &common.DirigentMetadata{
		Image:               image,
		Port:                portInt,
		Protocol:            "tcp",
		ScalingUpperBound:   upperScaleInt,
		ScalingLowerBound:   lowerScaleInt,
		IterationMultiplier: iterationMultiplierInt,
		IOPercentage:        0,
	}
}
