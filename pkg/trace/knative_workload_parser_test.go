package trace

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestConvertKnativeYAMLToDirigentMetadata(t *testing.T) {
	cfg := convertKnativeYamlToDirigentMetadata("test_data/service.yaml")

	assert.Equal(t, cfg.Image, "docker.io/cvetkovic/dirigent_trace_function:latest")
	assert.Equal(t, cfg.Port, 80)
	assert.Equal(t, cfg.Protocol, "tcp")
	assert.Equal(t, cfg.ScalingUpperBound, 200)
	assert.Equal(t, cfg.ScalingLowerBound, 0)
	assert.Equal(t, cfg.IterationMultiplier, 102)
	assert.Equal(t, cfg.IOPercentage, 50)
}
