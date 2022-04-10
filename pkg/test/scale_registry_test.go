package test

import (
	"testing"

	mc "github.com/eth-easl/loader/pkg/metric"
	"github.com/stretchr/testify/assert"
)

func TestGetColdStartCount(t *testing.T) {
	registry := mc.ScaleRegistry{}

	/** Initialisation */
	records := []mc.ScaleRecord{
		//* Scale up NOT from 0.
		{Deployment: "func-1", ActualScale: 0},
		//* Scale up from 0.
		{Deployment: "func-2", ActualScale: 0},
		//* Haven't scaled.
		{Deployment: "func-2", ActualScale: 0},
	}
	registry.Init(records)

	assert.Equal(t, 0, registry.UpdateAndGetColdStartCount(records))
	/** Cold start. */
	records = []mc.ScaleRecord{
		{Deployment: "func-1", ActualScale: 10},
	}
	assert.Equal(t, 1, registry.UpdateAndGetColdStartCount(records))

	/** Mixing cold start and normal scaling up. */
	records = []mc.ScaleRecord{
		//* Scale up NOT from 0.
		{Deployment: "func-1", ActualScale: 100},
		//* Scale up from 0.
		{Deployment: "func-2", ActualScale: 100},
		//* Haven't scaled.
		{Deployment: "func-2", ActualScale: 0},
	}
	assert.Equal(t, 1, registry.UpdateAndGetColdStartCount(records))

	//* Scale down to 0.
	records = []mc.ScaleRecord{
		{Deployment: "func-1", ActualScale: 0},
		{Deployment: "func-2", ActualScale: 0},
	}
	assert.Equal(t, 0, registry.UpdateAndGetColdStartCount(records))

	/** All cold starts */
	records = []mc.ScaleRecord{
		{Deployment: "func-1", ActualScale: 200},
		{Deployment: "func-2", ActualScale: 200},
		{Deployment: "func-3", ActualScale: 200},
	}
	assert.Equal(t, 3, registry.UpdateAndGetColdStartCount(records))

}
