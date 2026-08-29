package cluster

import (
	"strings"
	"testing"
)

func TestWorkerMaxPodsCommandIsIdempotent(t *testing.T) {
	command := workerMaxPodsCommand(360)
	for _, required := range []string{
		"sed -i",
		"maxPods:",
		"360",
		"tee -a /var/lib/kubelet/config.yaml",
	} {
		if !strings.Contains(command, required) {
			t.Fatalf("maxPods command missing %q: %s", required, command)
		}
	}
}

func TestValidateWorkerCapacityRequiresReadyTarget(t *testing.T) {
	ready := `{"items":[{"metadata":{"name":"worker"},"status":{"allocatable":{"pods":"360"},"conditions":[{"type":"Ready","status":"True"}]}}]}`
	if err := validateWorkerCapacity(ready, 360, 1); err != nil {
		t.Fatalf("accepted Ready worker failed validation: %v", err)
	}
	if err := validateWorkerCapacity(ready, 361, 1); err == nil {
		t.Fatal("accepted worker below target capacity")
	}
	notReady := `{"items":[{"metadata":{"name":"worker"},"status":{"allocatable":{"pods":"360"},"conditions":[{"type":"Ready","status":"False"}]}}]}`
	if err := validateWorkerCapacity(notReady, 360, 1); err == nil {
		t.Fatal("accepted worker that is not Ready")
	}
}
