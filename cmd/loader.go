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

package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/vhive-serverless/loader/pkg/generator"

	"golang.org/x/exp/slices"

	"github.com/vhive-serverless/loader/pkg/common"
	"github.com/vhive-serverless/loader/pkg/config"
	"github.com/vhive-serverless/loader/pkg/driver"
	"github.com/vhive-serverless/loader/pkg/trace"

	log "github.com/sirupsen/logrus"
	tracer "github.com/vhive-serverless/vSwarm/utils/tracing/go"
)

const (
	zipkinAddr = "http://localhost:9411/api/v2/spans"
)

var (
	configPath                = flag.String("config", "cmd/config_knative_trace.json", "Path to loader configuration file")
	failurePath               = flag.String("failureConfig", "cmd/failure.json", "Path to the failure configuration file")
	verbosity                 = flag.String("verbosity", "info", "Logging verbosity - choose from [info, debug, trace]")
	iatGeneration             = flag.Bool("iatGeneration", false, "Generate IATs only or run invocations as well")
	iatFromFile               = flag.Bool("generated", false, "True if iats were already generated")
	dryRun                    = flag.Bool("dryRun", false, "Dry run mode - do not deploy functions or generate invocations")
	e2AdmissionWorkload       = flag.String("e2-admission-workload", "", "E2 canonical workload to admit before acquisition")
	e2AdmissionReplicas       = flag.Int("e2-admission-replicas", 0, "E2 expected Ready replicas per function")
	e2AdmissionOutput         = flag.String("e2-admission-output", "", "E2 admission evidence output prefix")
	e2AcquisitionMarker       = flag.String("e2-acquisition-marker", "", "E2 acquisition-start marker path")
	e2AdmissionTimeoutSeconds = flag.Int("e2-admission-timeout-seconds", 300, "E2 admission stabilization timeout")
)

func init() {
	flag.Parse()

	log.SetFormatter(&log.TextFormatter{
		TimestampFormat: time.StampMilli,
		FullTimestamp:   true,
	})
	log.SetOutput(os.Stdout)

	switch *verbosity {
	case "debug":
		log.SetLevel(log.DebugLevel)
	case "trace":
		log.SetLevel(log.TraceLevel)
	default:
		log.SetLevel(log.InfoLevel)
	}
}

func main() {
	cfg := config.ReadConfigurationFile(*configPath)
	if cfg.EnableZipkinTracing {
		// TODO: how not to exclude Zipkin spans here? - file a feature request
		log.Warnf("Zipkin tracing has been enabled. This will exclude Istio spans from the Zipkin traces.")
		shutdown, err := tracer.InitBasicTracer(zipkinAddr, "loader")
		if err != nil {
			log.Print(err)
		}
		defer shutdown()
	}
	if cfg.ExperimentDuration < 1 {
		log.Fatal("Runtime duration should be longer, at least a minute.")
	}

	supportedPlatforms := []string{
		common.PlatformKnative,
		common.PlatformOpenWhisk,
		common.PlatformAWSLambda,
		common.PlatformDirigent,
		common.PlatformAzureFunctions,
		common.PlatformURL,
		common.PlatformNexus,
	}
	if !slices.Contains(supportedPlatforms, cfg.Platform) {
		log.Fatal("Unsupported platform!")
	}

	if cfg.Platform == common.PlatformKnative {
		common.CheckCPULimit(cfg.CPULimit)
	}

	if cfg.TracePath == "RPS" {
		runRPSMode(&cfg, *iatFromFile, *iatGeneration)
	} else {
		runTraceMode(&cfg, *iatFromFile, *iatGeneration)
	}
}

func determineDurationToParse(runtimeDuration int, warmupDuration int) int {
	result := 0

	if warmupDuration > 0 {
		result += warmupDuration // warmup
	}

	result += runtimeDuration // actual experiment

	return result
}

func parseIATDistribution(cfg *config.LoaderConfiguration) (common.IatDistribution, bool) {
	switch cfg.IATDistribution {
	case "exponential":
		return common.Exponential, false
	case "exponential_shift":
		return common.Exponential, true
	case "uniform":
		return common.Uniform, false
	case "uniform_shift":
		return common.Uniform, true
	case "equidistant":
		return common.Equidistant, false
	default:
		log.Fatal("Unsupported IAT distribution.")
	}

	return common.Exponential, false
}

func parseYAMLSpecification(cfg *config.LoaderConfiguration) string {
	switch cfg.YAMLSelector {
	case "container":
		return "workloads/container/trace_func_go.yaml"
	case "firecracker":
		return "workloads/firecracker/trace_func_go.yaml"
	case "nexus":
		return "workloads/nexus/mock_func.yaml"
	default:
		if cfg.Platform != common.PlatformDirigent && cfg.Platform != common.PlatformAzureFunctions {
			log.Fatal("Invalid 'YAMLSelector' parameter.")
		}
	}

	return ""
}

func parseTraceGranularity(cfg *config.LoaderConfiguration) common.TraceGranularity {
	switch cfg.Granularity {
	case "minute":
		return common.MinuteGranularity
	case "second":
		return common.SecondGranularity
	default:
		log.Fatal("Invalid trace granularity parameter.")
	}

	return common.MinuteGranularity
}

type e2AdmissionGate struct {
	workload, outputPrefix, marker   string
	expectedReplicas, timeoutSeconds int
}

func (g e2AdmissionGate) probe() error {
	if g.workload == "" || g.outputPrefix == "" || g.marker == "" || g.expectedReplicas <= 0 || g.timeoutSeconds <= 0 {
		return fmt.Errorf("incomplete E2 admission gate configuration")
	}
	if err := os.MkdirAll(filepath.Dir(g.outputPrefix), 0755); err != nil {
		return err
	}
	pollPath := g.outputPrefix + "-poll.txt"
	_ = os.Remove(g.outputPrefix + "-deployments.json")
	_ = os.Remove(g.outputPrefix + ".csv")
	_ = os.Remove(g.outputPrefix + "-validation.txt")
	deadline := time.Now().Add(time.Duration(g.timeoutSeconds) * time.Second)
	for attempt := 1; ; attempt++ {
		deploymentsPath := g.outputPrefix + "-deployments.json"
		started := time.Now().UTC().Format(time.RFC3339)
		output, kubectlErr := exec.Command("kubectl", "get", "deployments", "-n", "default", "-o", "json").Output()
		_ = os.WriteFile(deploymentsPath, output, 0644)
		validationOutput := ""
		var validationErr error
		if kubectlErr == nil {
			_ = os.Remove(g.outputPrefix + ".csv")
			validator := exec.Command("python3", "experiment/e2/validate_admission.py",
				"--deployments", deploymentsPath, "--workload", g.workload,
				"--expected-replicas", fmt.Sprint(g.expectedReplicas), "--output", g.outputPrefix+".csv")
			var validationBytes []byte
			validationBytes, validationErr = validator.CombinedOutput()
			validationOutput = strings.TrimSpace(string(validationBytes))
		} else {
			validationErr = kubectlErr
			validationOutput = kubectlErr.Error()
		}
		pollLine := fmt.Sprintf("attempt=%d utc=%s kubectl_status=%t validation_status=%t detail=%s\n", attempt, started, kubectlErr == nil, validationErr == nil, validationOutput)
		pollHandle, err := os.OpenFile(pollPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return err
		}
		_, _ = pollHandle.WriteString(pollLine)
		_ = pollHandle.Close()
		if validationErr == nil {
			_ = os.WriteFile(g.outputPrefix+"-validation.txt", []byte(validationOutput+"\n"), 0644)
			return nil
		}
		_ = os.WriteFile(g.outputPrefix+"-validation.txt", []byte(validationOutput+"\n"), 0644)
		if time.Now().After(deadline) {
			return fmt.Errorf("E2 admission did not stabilize within %ds: %s", g.timeoutSeconds, validationOutput)
		}
		time.Sleep(1 * time.Second)
	}
}

func (g e2AdmissionGate) mark() error {
	if err := os.MkdirAll(filepath.Dir(g.marker), 0755); err != nil {
		return err
	}
	return os.WriteFile(g.marker, []byte("acquisition_started=true\nutc="+time.Now().UTC().Format(time.RFC3339)+"\n"), 0644)
}

func configureE2AdmissionGate(d *driver.Driver) {
	if *e2AdmissionWorkload == "" {
		return
	}
	gate := e2AdmissionGate{workload: *e2AdmissionWorkload, outputPrefix: *e2AdmissionOutput,
		marker: *e2AcquisitionMarker, expectedReplicas: *e2AdmissionReplicas, timeoutSeconds: *e2AdmissionTimeoutSeconds}
	d.PreAcquisition = gate.probe
	d.MarkAcquisitionStart = gate.mark
}

func runTraceMode(cfg *config.LoaderConfiguration, readIATFromFile bool, writeIATsToFile bool) {
	durationToParse := determineDurationToParse(cfg.ExperimentDuration, cfg.WarmupDuration)
	yamlPath := parseYAMLSpecification(cfg)
	var functions []*common.Function
	var traceParser trace.Parser

	// Azure trace parsing
	if !cfg.VSwarm {
		traceParser = trace.NewAzureParser(cfg.TracePath, durationToParse, yamlPath)
	} else {
		traceParser = trace.NewMapperParser(cfg.TracePath, durationToParse)
	}
	if cfg.Platform == common.PlatformNexus {
		traceParser = trace.NewNexusParser(cfg.TracePath, durationToParse, yamlPath)
	}

	functions = traceParser.Parse()
	// Dirigent metadata parsing
	dirigentMetadataParser := trace.NewDirigentMetadataParser(cfg.TracePath, functions, yamlPath, cfg.Platform)
	dirigentMetadataParser.Parse()

	log.Infof("Traces contain the following %d functions:\n", len(functions))
	for _, function := range functions {
		fmt.Printf("\t%s\n", function.Name)
	}

	iatType, shiftIAT := parseIATDistribution(cfg)

	experimentDriver := driver.NewDriver(&config.Configuration{
		LoaderConfiguration:  cfg,
		FailureConfiguration: config.ReadFailureConfiguration(*failurePath),

		// loads dirigent config only if the platform is 'dirigent'
		DirigentConfiguration: config.ReadDirigentConfig(cfg),

		IATDistribution:  iatType,
		ShiftIAT:         shiftIAT,
		TraceGranularity: parseTraceGranularity(cfg),
		TraceDuration:    durationToParse,

		TestMode: false,

		Functions: functions,
	})
	configureE2AdmissionGate(experimentDriver)

	log.Infof("Using %s as a service YAML specification file.\n", yamlPath)

	experimentDriver.GenerateSpecification()
	experimentDriver.ReadOrWriteFileSpecification(writeIATsToFile, readIATFromFile)

	if experimentDriver.Configuration.WithWarmup() {
		trace.DoStaticTraceProfiling(experimentDriver.Configuration.Functions, cfg.FixedReplicaCount)
	}

	// Skip experiments execution during dry run mode
	if *dryRun {
		return
	}

	if err := experimentDriver.RunExperiment(); err != nil {
		log.Fatal(err)
	}
}

func runRPSMode(cfg *config.LoaderConfiguration, readIATFromFile bool, writeIATsToFile bool) {
	experimentDuration := determineDurationToParse(cfg.ExperimentDuration, cfg.WarmupDuration)
	yamlPath := parseYAMLSpecification(cfg)

	rpsTarget := cfg.RpsTarget
	coldStartPercentage := cfg.RpsColdStartRatioPercentage

	warmStartRPS := rpsTarget * (100 - coldStartPercentage) / 100
	coldStartRPS := rpsTarget * coldStartPercentage / 100

	iatType, shiftIAT := parseIATDistribution(cfg)

	warmFunctions := make([]common.IATArray, cfg.RpsFunctionCount)
	warmStartCounts := make([][]int, cfg.RpsFunctionCount)
	for i := 0; i < cfg.RpsFunctionCount; i++ {
		warmFunction, warmStartCount := generator.GenerateWarmStartFunction(i, experimentDuration, parseTraceGranularity(cfg), warmStartRPS, iatType, shiftIAT, cfg.Seed)
		warmFunctions[i] = warmFunction
		if len(warmFunctions[i]) > 0 {
			warmFunctions[i][0] = (float64(i) / float64(cfg.RpsFunctionCount)) * (1000000.0 / float64(warmStartRPS))
		}
		warmStartCounts[i] = warmStartCount
	}

	coldFunctions, coldStartCount := generator.GenerateColdStartFunctions(experimentDuration, parseTraceGranularity(cfg), coldStartRPS, cfg.RpsCooldownSeconds, iatType, shiftIAT, cfg.Seed)

	// loads dirigent config only if the platform is 'dirigent'
	dirigentConfig := config.ReadDirigentConfig(cfg)

	experimentDriver := driver.NewDriver(&config.Configuration{
		LoaderConfiguration: cfg,
		TraceDuration:       experimentDuration,

		DirigentConfiguration: dirigentConfig,

		Functions: generator.CreateRPSFunctions(cfg, dirigentConfig, warmFunctions, warmStartCounts, coldFunctions, coldStartCount, yamlPath),
	})
	configureE2AdmissionGate(experimentDriver)

	if experimentDriver.Configuration.WithWarmup() {
		trace.DoStaticTraceProfiling(experimentDriver.Configuration.Functions, cfg.FixedReplicaCount)
	}

	// Skip experiments execution during dry run mode
	if *dryRun {
		return
	}

	experimentDriver.ReadOrWriteFileSpecification(writeIATsToFile, readIATFromFile)
	if err := experimentDriver.RunExperiment(); err != nil {
		log.Fatal(err)
	}
}
