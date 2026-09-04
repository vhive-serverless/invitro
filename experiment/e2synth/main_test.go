package main

import (
	"encoding/csv"
	"os"
	"testing"
)

func TestStandaloneDefaults(t *testing.T) {
	full := defaultOptions("collect")
	if full.replicas != 320 || full.warmupMinutes != 2 || full.measurementMinutes != 3 || full.repetitions != 1 {
		t.Fatalf("full defaults changed: %+v", full)
	}
	if full.modes != "invm-py,invm-js,invm-go,hosttcp-go,nexus-py,nexus-js,nexus-go,nexus-rdma-py,nexus-rdma-go" {
		t.Fatalf("mode contract changed: %s", full.modes)
	}
	calibration := defaultOptions("calibrate")
	if calibration.modes != "invm-py" || calibration.steps != 20 || calibration.minutesPerStep != 1 {
		t.Fatalf("calibration defaults changed: %+v", calibration)
	}
	smoke := defaultOptions("smoke")
	if smoke.replicas != 2 || smoke.warmupMinutes != 2 || smoke.measurementMinutes != 1 {
		t.Fatalf("smoke defaults changed: %+v", smoke)
	}
}

func TestSmokeInputsCoverCanonicalPayloads(t *testing.T) {
	averages, reference, cleanup, err := makeSmokeInputs()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	for _, path := range []string{averages, reference} {
		handle, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		rows, err := csv.NewReader(handle).ReadAll()
		handle.Close()
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 11 {
			t.Fatalf("%s has %d rows, want header plus ten payloads", path, len(rows))
		}
	}
}
