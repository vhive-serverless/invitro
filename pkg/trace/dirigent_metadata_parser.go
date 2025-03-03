package trace

import (
	"encoding/json"
	log "github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/pkg/common"
	"os"
	"strings"
)

type DirigentMetadataParser struct {
	directoryPath string
	functions     []*common.Function
	yamlPath      string
	platform      string
}

func NewDirigentMetadataParser(directoryPath string, functions []*common.Function, yamlPath string, platform string) *DirigentMetadataParser {
	return &DirigentMetadataParser{
		directoryPath: directoryPath,
		functions:     functions,
		yamlPath:      yamlPath,
		platform:      platform,
	}
}

func readDirigentMetadataJSON(traceFile string, platform string) *[]common.DirigentMetadata {
	if !strings.Contains(platform, common.PlatformDirigent) {
		return nil
	}

	log.Infof("Parsing Dirigent metadata: %s", traceFile)

	data, err := os.ReadFile(traceFile)
	if err != nil {
		log.Error("Failed to read Dirigent trace file.")
		return nil
	}

	var metadata []common.DirigentMetadata
	err = json.Unmarshal(data, &metadata)
	if err != nil {
		log.Fatal("Failed to parse trace runtime specification.")
	}

	return &metadata
}

func (dmp *DirigentMetadataParser) Parse() {
	dirigentPath := dmp.directoryPath + "/dirigent.json"
	dirigentMetadata := readDirigentMetadataJSON(dirigentPath, dmp.platform)

	var dirigentMetadataByHashFunction map[string]*common.DirigentMetadata
	if dirigentMetadata != nil {
		dirigentMetadataByHashFunction = createDirigentMetadataMap(dirigentMetadata)
	}

	for _, function := range dmp.functions {
		if dirigentMetadata != nil {
			function.DirigentMetadata = dirigentMetadataByHashFunction[function.InvocationStats.HashFunction]
		} else if strings.Contains(strings.ToLower(dmp.platform), common.PlatformKnative) {
			// values are not used for Knative so they are irrelevant
			function.DirigentMetadata = convertKnativeYamlToDirigentMetadata(dmp.yamlPath)
		}
	}
}
