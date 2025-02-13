package trace

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/pkg/common"
)

type MapperTraceParser struct {
	DirectoryPath         string
	duration              int
	functionNameGenerator *rand.Rand
}

type DeploymentInfo struct {
	YamlLocation      string
	PredeploymentPath []string
}

type JSONParser map[string]DeploymentInfo

func NewMapperParser(directoryPath string, totalDuration int) *MapperTraceParser {
	return &MapperTraceParser{
		DirectoryPath: directoryPath,

		duration:              totalDuration,
		functionNameGenerator: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (p *MapperTraceParser) extractFunctions(mapperOutput map[string]map[string]string, deploymentInfo JSONParser, dirPath string) []*common.Function {
	var result []*common.Function

	invocations := parseInvocationTrace(dirPath+"/invocations.csv", p.duration)
	runtime := parseRuntimeTrace(dirPath + "/durations.csv")
	memory := parseMemoryTrace(dirPath + "/memory.csv")

	runtimeByHashFunction := createRuntimeMap(runtime)
	memoryByHashFunction := createMemoryMap(memory)

	for i := 0; i < len(*invocations); i++ {
		invocationStats := (*invocations)[i]
		hashFunction := invocationStats.HashFunction
		proxyFunction := mapperOutput[hashFunction]["proxy-function"]
		yamlPath := deploymentInfo[proxyFunction].YamlLocation
		predeploymentPaths := deploymentInfo[proxyFunction].PredeploymentPath
		function := &common.Function{
			Name: fmt.Sprintf("%s-%d-%d", proxyFunction, i, p.functionNameGenerator.Uint64()),

			InvocationStats:   &invocationStats,
			RuntimeStats:      runtimeByHashFunction[hashFunction],
			MemoryStats:       memoryByHashFunction[hashFunction],
			YAMLPath:          yamlPath,
			PredeploymentPath: predeploymentPaths,
		}

		result = append(result, function)
	}

	return result
}

func (p *MapperTraceParser) Parse() []*common.Function {
	var functions []*common.Function
	var mapperOutput map[string]map[string]string // HashFunction mapped to vSwarm function yaml.
	var deploymentInfo JSONParser
	// Read the deployment info file for yaml locations and predeployment commands if any
	deploymentInfoFile, err := os.ReadFile("test_data/test_deploy_info.json")
	if err != nil {
		wd, _ := os.Getwd()
		deploymentInfoFile, err = os.ReadFile(wd + "/workloads/container/yamls/deploy_info.json")
		if err != nil {
			log.Warn("No deployment info file")
		}
	}

	err = json.Unmarshal(deploymentInfoFile, &deploymentInfo)
	if err != nil {
		log.Warn("Failed to unmarshal deployment info file")
	}

	mapperFile, err := os.ReadFile(p.DirectoryPath + "/mapper_output.json")
	if err != nil {
		traces, err := os.ReadDir(p.DirectoryPath)
		if err != nil {
			log.Warn("No mapper output file")
		}

		for _, trace := range traces {
			traceName := trace.Name()
			mapperFile, err = os.ReadFile(p.DirectoryPath + "/" + traceName + "/mapper_output.json")
			if err != nil {
				log.Warn("Directory has no mapper_output")
				continue
			}
			err = json.Unmarshal(mapperFile, &mapperOutput)
			if err != nil {
				log.Warn("Failed to unmarshal mapper output file")
			}
			result := p.extractFunctions(mapperOutput, deploymentInfo, p.DirectoryPath+"/"+traceName)
			functions = append(functions, result...)
		}

		return functions
	}

	err = json.Unmarshal(mapperFile, &mapperOutput)
	if err != nil {
		log.Warn("Failed to unmarshal mapper output file")
	}

	functions = p.extractFunctions(mapperOutput, deploymentInfo, p.DirectoryPath)

	return functions
}
