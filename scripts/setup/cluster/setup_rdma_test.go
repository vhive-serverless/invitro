package cluster

import (
	"strings"
	"testing"
)

func TestRDMAPayloadProvisionCommandsPopulateCanonicalTreesAndVerifyMapper(t *testing.T) {
	commands := strings.Join(rdmaPayloadProvisionCommands(), "\n")
	for _, want := range []string{
		"~/khala/assets/nexus-benchmark-payload/input_payload/",
		"~/rdma-demo/assets/nexus-benchmark-payload/input_payload/",
		"~/khala/assets/nexus-benchmark-payload/test/",
		"~/rdma-demo/assets/nexus-benchmark-payload/test/",
		"~/khala/assets/synthetic-payload/",
		"~/rdma-demo/assets/synthetic-payload-input/",
		"mapper_scaled/part-00000.csv",
		"sha256sum",
	} {
		if !strings.Contains(commands, want) {
			t.Fatalf("RDMA payload provisioning commands missing %q:\n%s", want, commands)
		}
	}
}
