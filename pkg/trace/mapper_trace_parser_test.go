package trace
import (
	"testing"
	"strings"
)

func TestMapperParserWrapper(t *testing.T) {
	parser := NewMapperParser("test_data", 10)
	functions := parser.Parse()

	if len(functions) != 1 {
		t.Error("Invalid function array length.")
	}
	if !strings.HasPrefix(functions[0].Name, "cartservice") ||
		functions[0].InvocationStats == nil ||
		functions[0].YAMLPath != "workloads/container/yamls/online-shop/kn-cartservice.yaml" {
		t.Error("Unexpected results.")
	}
}