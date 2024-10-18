package config

import (
	"github.com/vhive-serverless/loader/pkg/common"
)

type Configuration struct {
	LoaderConfiguration  *LoaderConfiguration
	FailureConfiguration *FailureConfiguration

	IATDistribution  common.IatDistribution
	ShiftIAT         bool // shift the invocations inside minute
	TraceGranularity common.TraceGranularity
	TraceDuration    int // in minutes

	YAMLPath string
	TestMode bool

	Functions []*common.Function
}

func (c *Configuration) WithWarmup() bool {
	if c.LoaderConfiguration.WarmupDuration > 0 {
		return true
	} else {
		return false
	}
}
