package trace

import (
	"github.com/vhive-serverless/loader/pkg/common"
	"testing"
)

func TestDirigentParserFromJSON(t *testing.T) {
	functions := []*common.Function{
		{
			Name: "c13acdc7567b225971cef2416a3a2b03c8a4d8d154df48afe75834e2f5c59ddf",
			InvocationStats: &common.FunctionInvocationStats{
				HashFunction: "c13acdc7567b225971cef2416a3a2b03c8a4d8d154df48afe75834e2f5c59ddf",
			},
		},
		{
			Name: "ae8a1640fa932024f59b38a0b001808b5c64612bd60c6f3eb80ba9461ba2d091",
			InvocationStats: &common.FunctionInvocationStats{
				HashFunction: "ae8a1640fa932024f59b38a0b001808b5c64612bd60c6f3eb80ba9461ba2d091",
			},
		},
	}

	parser := NewDirigentMetadataParser("test_data", functions, "", common.PlatformDirigent)
	parser.Parse()

	d0 := functions[0].DirigentMetadata
	if !(d0.HashFunction == "c13acdc7567b225971cef2416a3a2b03c8a4d8d154df48afe75834e2f5c59ddf" &&
		d0.Image == "docker.io/vhiveease/relay:latest" &&
		d0.Port == 50000 &&
		d0.Protocol == "tcp" &&
		d0.ScalingUpperBound == 1 &&
		d0.ScalingLowerBound == 1 &&
		d0.IterationMultiplier == 80 &&
		d0.IOPercentage == 0 &&
		len(d0.EnvVars) == 1 &&
		len(d0.ProgramArgs) == 8) {

		t.Error("Unexpected results.")
	}

	d1 := functions[1].DirigentMetadata
	if !(d1.HashFunction == "ae8a1640fa932024f59b38a0b001808b5c64612bd60c6f3eb80ba9461ba2d091" &&
		d1.Image == "docker.io/cvetkovic/dirigent_grpc_function:latest" &&
		d1.Port == 80 &&
		d1.Protocol == "tcp" &&
		d1.ScalingUpperBound == 1 &&
		d1.ScalingLowerBound == 0 &&
		d1.IterationMultiplier == 80 &&
		d1.IOPercentage == 0 &&
		len(d1.EnvVars) == 0 &&
		len(d1.ProgramArgs) == 0) {

		t.Error("Unexpected results.")
	}
}

func TestDirigentMetadataFromKnativeYAML(t *testing.T) {
	functions := []*common.Function{
		{
			Name: "c13acdc7567b225971cef2416a3a2b03c8a4d8d154df48afe75834e2f5c59ddf",
			InvocationStats: &common.FunctionInvocationStats{
				HashFunction: "c13acdc7567b225971cef2416a3a2b03c8a4d8d154df48afe75834e2f5c59ddf",
			},
		},
		{
			Name: "ae8a1640fa932024f59b38a0b001808b5c64612bd60c6f3eb80ba9461ba2d091",
			InvocationStats: &common.FunctionInvocationStats{
				HashFunction: "ae8a1640fa932024f59b38a0b001808b5c64612bd60c6f3eb80ba9461ba2d091",
			},
		},
	}

	parser := NewDirigentMetadataParser("test_data", functions, "test_data/service.yaml", common.PlatformKnative)
	parser.Parse()

	d := functions[0].DirigentMetadata
	if !(d.Image == "docker.io/cvetkovic/dirigent_trace_function:latest" &&
		d.Port == 80 &&
		d.Protocol == "tcp" &&
		d.ScalingUpperBound == 200 &&
		d.ScalingLowerBound == 10 &&
		d.IterationMultiplier == 102 &&
		d.IOPercentage == 50 &&
		len(d.EnvVars) == 0 &&
		len(d.ProgramArgs) == 0) {

		t.Error("Unexpected results.")
	}
}
