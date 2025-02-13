package trace

import (
	"strings"
	"testing"
)

func TestMapperParserWrapper(t *testing.T) {
	parser := NewMapperParser("test_data", 10)
	functions := parser.Parse()

	if len(functions) != 1 {
		t.Error("Invalid function array length.")
	}
	if !strings.HasPrefix(functions[0].Name, "cartservice") ||
		functions[0].InvocationStats == nil ||
		functions[0].YAMLPath != "workloads/container/yamls/online-shop/kn-cartservice.yaml" ||
		len(functions[0].PredeploymentCommands) != 1 ||
		functions[0].PredeploymentCommands[0] != "kubectl apply -f workloads/container/yamls/online-shop/database.yaml" {
		t.Error("Unexpected results.")
	}
}
