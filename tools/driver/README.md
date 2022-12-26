# Experiment Driver

## Usage

### Configuring the Driver

The following fields in driverConfig.json can be configured:  

"username": Your username on Cloudlab, used to ssh into the loader node. Note that you will need an ssh agent configured
on the machine running the experiment driver.  
"experimentName": The name of the experiment, creates a directory under that name within the specified output directory.

"localTracePath": The path to the trace files on the machine running the experiment driver. The trace files need to be
called {n}_inv.csv, {n}_run.csv and {n}_mem.csv, with n being the number of functions. See further below for a more
detailed explanation.  

"loaderTracePath": The path at which trace files on the loader node should be stored, can remain unchanged.  
"loaderAddresses": The address(es) of the loader node(s), currently only one loader is supported.  

The driver will run multiple experiments sequentially with different traces. Currently, this is based on the number
of functions within the trace. For example, if you want to sweep traces with 50, 100, 150 and 200 functions, you
would put the trace files within one folder, with the invocation files named 50_inv.csv, 100_inv.csv, 150_inv.csv and
200_inv.csv. The durations and memory trace files should be named {n}_dur.csv and {n}_mem.csv, with n being the number
of functions. Then you would set the following parameters to sweep these trace files:  
"beginningFuncNum": 50  
"stepSizeFunc": 50  
"maxFuncNum": 200  
This will run a total of four experiments.

"experimentDuration": How long the experiment should last  
"warmupDuration": The duration of the warmup phase.  
"workerNodeNum": The number of worker nodes in the cluster  
"outputDir": The local output directory on the machine running the experiment driver, this is where experiment results
will be saved.  
"YAMLSelector": Whether to use containers or Firecracker.  
"IATDistribution": The IAT distribution that the loader will use.  
"loaderOutputPath": The output path for the experiment results on the loader node.  
"partiallyPanic": Whether to enable partially panic mode.  
"EnableZipkinTracing": Whether to enable Zipkin Tracing.  
"EnableMetricsScrapping": Whether to enable Metrics Scraping.  
"separateIATGeneration": Whether to generate IATs before running the loader. Leave as false unless you are running
multiple loader nodes for one experiment (which is currently not supported).  

### Running the Experiment

```bash
$ go run experiment_driver.go -c driverConfig.json
```