# Loader configuration file format

| Parameter name               | Data type | Possible values                                                     | Default value       | Description                                                                                                                                                                                                                              |
|------------------------------|-----------|---------------------------------------------------------------------|---------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Seed                         | int64     | any                                                                 | 42                  | Seed for specification generator (for reproducibility)                                                                                                                                                                                   |
| Platform                     | string    | Knative, OpenWhisk, AWSLambda, Dirigent, Dirigent-Dandelion         | Knative             | The serverless platform the functions will be executed on                                                                                                                                                                                |
| DirigentConfigPath [^9]      | string    | N/A                                                                 | ""                  | Path to the Dirigent configuration file                                                                                                                                                                                                  |
| InvokeProtocol               | string    | grpc, http1, http2                                                  | N/A                 | Protocol to use to communicate with the sandbox                                                                                                                                                                                          |
| YAMLSelector                 | string    | wimpy, container, firecracker                                       | container           | Service YAML depending on sandbox type                                                                                                                                                                                                   |
| EndpointPort                 | int       | > 0                                                                 | 80                  | Port to be appended to the service URL                                                                                                                                                                                                   | 
| RpsTarget                    | int       | >= 0                                                                | 0                   | Number of requests per second to issue                                                                                                                                                                                                   | 
| RpsColdStartRatioPercentage  | int       | >= 0 && <= 100                                                      | 0                   | Percentage of cold starts out of specified RPS                                                                                                                                                                                           | 
| RpsCooldownSeconds [^6]      | int       | > 0                                                                 | 0                   | The time it takes for the autoscaler to downscale function (higher for higher RPS)                                                                                                                                                       |
| RpsImage                     | string    | N/A                                                                 | N/A                 | Function image to use for RPS experiments                                                                                                                                                                                                |
| RpsRuntimeMs                 | int       | >= 0                                                                | 0                   | Requested execution time                                                                                                                                                                                                                 |
| RpsMemoryMB                  | int       | >= 0                                                                | 0                   | Requested memory                                                                                                                                                                                                                         |
| RpsIterationMultiplier       | int       | >= 0                                                                | 0                   | Iteration multiplier for RPS mode                                                                                                                                                                                                        |
| TracePath [^1]               | string    | string                                                              | data/traces/example | Folder with Azure trace dimensions (invocations.csv, durations.csv, memory.csv) or "RPS"                                                                                                                                                 |
| Granularity                  | string    | minute, second                                                      | minute              | Granularity for trace interpretation[^2]                                                                                                                                                                                                 |
| OutputPathPrefix             | string    | any                                                                 | data/out/experiment | Results file(s) output path prefix                                                                                                                                                                                                       |
| IATDistribution              | string    | exponential, exponential_shift, uniform, uniform_shift, equidistant | exponential         | IAT distribution[^3]                                                                                                                                                                                                                     |
| CPULimit                     | string    | 1vCPU, GCP                                                          | 1vCPU               | Imposed CPU limits on worker containers (only applicable for 'Knative' platform)[^4]                                                                                                                                                     |
| ExperimentDuration           | int       | > 0                                                                 | 1                   | Experiment duration in minutes of trace to execute excluding warmup                                                                                                                                                                      |
| WarmupDuration               | int       | > 0                                                                 | 0                   | Warmup duration in minutes(disabled if zero)                                                                                                                                                                                             |
| PrepullMode                  | string    | all_sync, all_async, one_sync, one_async, none                      | none                | Prepull image before starting experiments sync or async                                                                                                                                                                                  |
| IsPartiallyPanic             | bool      | true/false                                                          | false               | Pseudo-panic-mode only in Knative                                                                                                                                                                                                        |
| EnableZipkinTracing          | bool      | true/false                                                          | false               | Show loader span in Zipkin traces                                                                                                                                                                                                        |
| EnableMetricsScrapping       | bool      | true/false                                                          | false               | Scrap cluster-wide metrics                                                                                                                                                                                                               |
| MetricScrapingPeriodSeconds  | int       | > 0                                                                 | 15                  | Period of Prometheus metrics scrapping                                                                                                                                                                                                   |
| GRPCConnectionTimeoutSeconds | int       | > 0                                                                 | 60                  | Timeout for establishing a gRPC connection                                                                                                                                                                                               |
| GRPCFunctionTimeoutSeconds   | int       | > 0                                                                 | 90                  | Maximum time given to function to execute[^5]                                                                                                                                                                                            |
| DAGMode                      | bool      | true/false                                                          | false               | Generates DAG workflows iteratively with functions in TracePath [^7]. Frequency and IAT of the DAG follows their respective entry function, while Duration and Memory of each function will follow their respective values in TracePath. |                            
| EnableDAGDataset             | bool      | true/false                                                          | true                | Generate width and depth from dag_structure.csv in TracePath[^8]                                                                                                                                                                         |
| Width                        | int       | > 0                                                                 | 2                   | Default width of DAG                                                                                                                                                                                                                     |
| Depth                        | int       | > 0                                                                 | 2                   | Default depth of DAG                                                                                                                                                                                                                     |

[^1]: To run RPS experiments replace the path with `RPS`.

[^2]: The second granularity feature interprets each column of the trace as a second, rather than as a minute, and
generates IAT for each second. This feature is useful for fine-grained and precise invocation scheduling in experiments
involving stable low load.

[^3]: `_shift` modifies the IAT generation in the following way: by default, generation will create first invocation in
the beginning of the minute, with `_shift` modifier, it will be shifted inside the minute to remove the burst of
invocations from all the functions.

[^4]: Limits are set by resource->limits->CPU in the service YAML. `1vCPU` means limit of 1CPU is set, at the same time
execution is also limited by the container concurrency limit of 1. `GCP` means limits are set to multiples of 1/12th of
vCPU, based on the memory consumption of the function according to
this [table](https://cloud.google.com/functions/pricing#compute_time) for Google Cloud Functions.

[^5]: Function can execute for at most 15 minutes as in AWS
Lambda; https://aws.amazon.com/about-aws/whats-new/2018/10/aws-lambda-supports-functions-that-can-run-up-to-15-minutes/

[^6] It is recommended that the first 10% of cold starts are discarded from the experiment results for low cold start
RPS.

[^7]: The generated DAGs consist of unique functions. The shape of each DAG is determined either ```Width,Depth``` or calculated based on ```EnableDAGDAtaset```.

[^8]: A [data sample](https://github.com/icanforce/Orion-OSDI22/blob/main/Public_Dataset/dag_structure.xlsx) of DAG structures has been created based on past Microsoft Azure traces. Width and Depth are determined based on probabilities of this sample.

[^9]: Required only when the Platform is `Dirigent`.

---

InVitro can cause failure on cluster manager components. To do so, please configure the `cmd/failure.json`. Make sure
that the node on which you run InVitro has SSH access to the target node.

| Parameter name | Description                                                                        |
|----------------|------------------------------------------------------------------------------------|
| FailureEnabled | Toggle to enable this feature                                                      |
| FailAt         | Time in seconds since the beginning of the experiment when to trigger a failure    | 
| FailComponent  | Which component to fail (choose from 'control_plane', 'data_plane', 'worker_node') |
| FailNode       | Which node(s) to fail (specify separated by blank space)                           |

---

# Dirigent configuration
| Parameter name           | Data type | Possible values                          | Default value | Description                                                              |
|--------------------------|-----------|------------------------------------------|---------------|--------------------------------------------------------------------------|
| Backend                  | string    | `containerd`, `firecracker`, `dandelion` | `containerd`  | The backend used in Dirigent                                             |
| DirigentControlPlaneIP   | string    | N/A                                      | N/A           | IP address of the Dirigent control plane (for function deployment)       |
| BusyLoopOnSandboxStartup | bool      | true/false                               | false         | Enable artificial delay on sandbox startup                               |
| AsyncMode                | bool      | true/false                               | false         | Enable asynchronous invocations in Dirigent                              |
| AsyncResponseURL         | string    | N/A                                      | N/A           | URL from which to collect invocation responses                           |
| AsyncWaitToCollectMin    | int       | >= 0                                     | 0             | Time after experiment ends after which to collect invocation results     |
| RpsDataSizeMB            | float64   | >= 0                                     | 0             | Amount of random data (same for all requests) to embed into each request |
| Workflow [^1]            | bool      | true/false                               | false         | Send workflow requests to Dirigent                                       | 
| WorkflowConfigPath [^2]  | string    | N/A                                      | N/A           | Path to the configuration file for the workflow requests (see below)     | 

[^1] Only supported for backend `dandelion`.

[^2] Required only when Workflow is set to true.

---

# Workflow configuration
| Parameter name | Data type           | Description                                       |
|----------------|---------------------|---------------------------------------------------|
| Name           | string              | Name to be used in the registration request.      |
| Functions      | []WorkflowFunction  | Functions used in the composition(s).             |
| Compositions   | []CompositionConfig | Compositions defined in the workflow description. |

### WorkflowFunction
| Parameter name | Data type | Description                               |
|----------------|-----------|-------------------------------------------|
| FunctionName   | string    | Function name used in the workflow.       |
| FunctionPath   | string    | Path to the binary located on the worker. |
| NumArgs        | int       | Number of input sets.                     |
| NumRets        | int       | Number of output sets.                    |

### CompositionConfig
| Parameter name | Data type  | Description                                                             |
|----------------|------------|-------------------------------------------------------------------------|
| Name           | string     | Composition name.                                                       |
| InData [^1]    | [][]string | First dimension are the input sets, second one are the items (per set). |

[^1] Prepend `%path=` to load the content from a local file path. Used empty string to use an empty input item.
