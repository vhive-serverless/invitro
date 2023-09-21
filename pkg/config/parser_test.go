/*
 * MIT License
 *
 * Copyright (c) 2023 EASL and the vHive community
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 */

package config

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestConfigParser(t *testing.T) {
	wd, _ := os.Getwd()

	var pathToConfigFile = wd
	if strings.HasSuffix(wd, "pkg/config") {
		pathToConfigFile += "/../../"
	}
	pathToConfigFile += "cmd/config.json"

	fmt.Println(pathToConfigFile)

	config := ReadConfigurationFile(pathToConfigFile)

	if config.Seed != 42 ||
		config.YAMLSelector != "container" ||
		config.EndpointPort != 80 ||
		!strings.HasPrefix(config.TracePath, "data/traces") ||
		!strings.HasPrefix(config.OutputPathPrefix, "data/out/experiment") ||
		config.Granularity != "minute" ||
		config.IATDistribution != "exponential" ||
		config.ExperimentDuration != 2 ||
		config.WarmupDuration != 0 ||
		config.IsPartiallyPanic != false ||
		config.EnableZipkinTracing != false ||
		config.EnableMetricsScrapping != false ||
		config.MetricScrapingPeriodSeconds != 15 ||
		config.GRPCConnectionTimeoutSeconds != 60 ||
		config.GRPCFunctionTimeoutSeconds != 900 {

		t.Error("Unexpected configuration read.")
	}
}
