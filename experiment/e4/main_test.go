package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vhive-serverless/loader/experiment/eval"
)

func TestFrozenPlanAlternatesModeOrder(t *testing.T) {
	topology := copyTopology(t)
	o := options{common: eval.Config{Profile: eval.Profile4, TopologyConfig: topology,
		MinioEndpoint: "http://" + eval.CanonicalMinioHost, ResultRoot: "/mnt/resources/nexus-evaluation/test/e4"},
		workloads: join(workloads), modes: join(modes), countsText: "1,2,4,6,8,10,20,30,40", warmup: 1, sampleSeconds: 10}
	plan, _, _, err := makePlan(o)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan) != 22 {
		t.Fatalf("cells = %d, want 22", len(plan))
	}
	if plan[0].Mode != "invm-py" || plan[1].Mode != "nexus-py" || plan[2].Mode != "nexus-py" || plan[3].Mode != "invm-py" {
		t.Fatalf("mode order not alternated: %#v", plan[:4])
	}
}

func TestSmokePlanIsTwoCells(t *testing.T) {
	o := options{common: eval.Config{Profile: eval.Profile4, TopologyConfig: copyTopology(t),
		MinioEndpoint: "http://" + eval.CanonicalMinioHost, ResultRoot: "/mnt/resources/nexus-evaluation/test/e4-smoke"},
		workloads: "helloworld", modes: join(modes), countsText: "1,2", warmup: 1, sampleSeconds: 10, smoke: true}
	plan, _, _, err := makePlan(o)
	if err != nil || len(plan) != 2 {
		t.Fatalf("smoke plan = %d, %v", len(plan), err)
	}
}

func copyTopology(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "topology.json")
	data, err := os.ReadFile("../../scripts/setup/configs/node_setup_base_1.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func join(values []string) string { return stringsJoin(values, ",") }

func stringsJoin(values []string, separator string) string {
	result := ""
	for index, value := range values {
		if index > 0 {
			result += separator
		}
		result += value
	}
	return result
}
