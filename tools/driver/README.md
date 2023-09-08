# Experiment Driver

## Platform Setup

One node (or personal machine) to run the experiment driver. One cluster consisting of one loader node connected to 
one or multiple worker nodes via passwordless ssh (as in Cloudlab). The node running the experiment driver needs 
to have an ssh agent running to connect to the loader node. The trace files should be present on the node running
the experiment driver. In the first phase, the driver will connect to the loader node and transfer the relevant
trace and configuration files. It will then start the experiment, during which the loader node will send invocations
to the worker node(s) and collect the results. The driver will then transfer those result files to its own node. After
this it will send a signal to the loader node to clean up the state of the cluster, to prepare it for the next
experiment.

## Usage

### Configuring the Driver

The following fields in driverConfig.json can be configured:  

"username": Your username on the target Cloudlab cluster, used to ssh into the loader node. 
Note that you will need an ssh agent configured on the machine running the experiment driver.  
"experimentName": The name of the experiment, creates a directory under that name within the specified output directory.

"localTracePath": The path to the trace files on the machine running the experiment driver. The trace files need to be
called {n}_inv.csv, {n}_run.csv and {n}_mem.csv, with n being the number of functions. See further below for a more
detailed explanation.  

"loaderTracePath": The path at which trace files on the loader node should be stored, can remain unchanged.  
"loaderAddresses": The hostname(s) of the loader node(s), for example "hp004.utah.cloudlab.us". 
Currently only one loader is supported.  

The driver runs multiple experiments sequentially with different traces. Currently, this is based on the number
of functions within the trace. For example, if you want to sweep traces with 50, 100, 150 and 200 functions, you
would put the trace files within one folder, with the invocation files named 50_inv.csv, 100_inv.csv, 150_inv.csv and
200_inv.csv. The durations and memory trace files should be named {n}_dur.csv and {n}_mem.csv, with n being the number
of functions. Then you would set the following parameters to sweep these trace files:  
"beginningFuncNum": 50  
"stepSizeFunc": 50  
"maxFuncNum": 200  
This will run a total of four experiments.

"experimentDuration": How long the measurement phase of the experiment should last. The total duration of the experiment
is then the duration of the measurement phase + the duration of the warmup phase + 1 minute for the profiling phase.  
"warmupDuration": The duration of the warmup phase.  
"workerNodeNum": The number of worker nodes in the cluster  
"outputDir": The local output directory on the machine running the experiment driver, this is where experiment results
will be saved.  
"YAMLSelector": Which yaml specification to use, supported values are 
"[wimpy](https://github.com/vhive-serverless/loader/blob/main/workloads/container/wimpy.yaml)", 
"[container](https://github.com/vhive-serverless/loader/blob/main/workloads/container/trace_func_go.yaml)" and 
"[firecracker](https://github.com/vhive-serverless/loader/blob/main/workloads/firecracker/trace_func_go.yaml)".  
"IATDistribution": The IAT distribution that the loader will use when sending invocations to the worker node(s).
Supported values are "exponential", "uniform" and "equidistant".  
"loaderOutputPath": The output path for the experiment results on the loader node.  
"partiallyPanic": Whether to enable partially panic mode. This modifies the
[panic-window-percentage](https://knative.dev/docs/serving/autoscaling/kpa-specific/#panic-window) from the default
value of 10.0 to the maximum value of 100.0, and it modifies the 
[panic-threshold-percentage](https://knative.dev/docs/serving/autoscaling/kpa-specific/#panic-mode-threshold)
from the default value of 200.0 to the maximum value of 1000.0.  
"EnableZipkinTracing": Whether to enable Zipkin Tracing. Calls 
[InitBasicTracer](https://github.com/vhive-serverless/vHive/blob/030c56e16a28ca431d3dfe0a21ff63f55912ef5a/utils/tracing/go/tracing.go#L81) 
to initialise an OpenTelemetry tracer.  
"EnableMetricsScrapping": Whether to enable Metrics Scraping.  
"MetricScrapingPeriodSeconds": The interval at which metrics scraping occurs, in seconds.  
"separateIATGeneration": Whether to generate IATs before running the loader. Leave as false unless you are running
multiple loader nodes for one experiment (which is currently not supported).
See issue #88.  
"AutoscalingMetric": Which autoscaling metric Knative should use. Default is "concurrency", supported values are
"concurrency" and "rps". See [Knative Doc](https://knative.dev/docs/serving/autoscaling/autoscaling-metrics/) for more 
details.  

### Running the Experiment

```bash
$ go run experiment_driver.go -c driverConfig.json
```
