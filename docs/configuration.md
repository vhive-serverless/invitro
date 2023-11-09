# Loader configuration file format

| Parameter name               | Data type | Possible values                                                                                  | Default value       | Description                                                            |
|------------------------------|-----------|--------------------------------------------------------------------------------------------------|---------------------|------------------------------------------------------------------------|
| Seed                         | int64     | any                                                                                              | 42                  | Seed for specification generator (for reproducibility)                 |
| Platform                     | string    | Knative, Knative-RPS, OpenWhisk, OpenWhisk-RPS, AWSLambda, AWSLambda-RPS, Dirigent, Dirigent-RPS | Knative             | The serverless platform the functions will be executed on              |
| YAMLSelector                 | string    | wimpy, container, firecracker                                                                    | container           | Service YAML depending on sandbox type                                 |
| EndpointPort                 | int       | > 0                                                                                              | 80                  | Port to be appended to the service URL                                 |
| RpsTarget                    | int       | > 0                                                                                              | 2                   | RPS-mode target load                                                   |
| RpsColdStartRatioPercentage  | int       | >= 0 and <= 100                                                                                  | 50                  | RPS-mode cold/warm start ratio                                         |
| CooldownSeconds              | int       | > 0                                                                                              | 10                  | RPS-mode target load                                                   |
| RpsRuntimeMs                 | int       | > 0                                                                                              | 10                  | RPS-mode function runtime                                              |
| RpsMemoryMB                  | int       | > 0                                                                                              | 2048                | RPS-mode function memory footprint                                     |
| RpsIterationMultiplier       | int       | > 0                                                                                              | 80                  | RPS-mode SQRT iterations                                               |
| TracePath                    | string    | string                                                                                           | data/traces         | Folder with Azure trace dimensions (invocations.csv, durations.csv,    |
| memory.csv)                  |           |                                                                                                  |                     |                                                                        |
| Granularity                  | string    | minute, second                                                                                   | minute              | Granularity for trace                                                  |
| interpretation[^1]           |           |                                                                                                  |                     |                                                                        |
| OutputPathPrefix             | string    | any                                                                                              | data/out/experiment | Results file(s) output path prefix                                     |
| IATDistribution              | string    | exponential, exponential_shift, uniform, uniform_shift, equidistant                              | exponential         | IAT                                                                    |
| distribution[^2]             |           |                                                                                                  |                     |                                                                        |
| CPULimit                     | string    | 1vCPU, GCP                                                                                       | 1vCPU               | Imposed CPU limits on worker containers (only applicable for 'Knative' |
| platform)[^3]                |           |                                                                                                  |                     |                                                                        |
| ExperimentDuration           | int       | > 0                                                                                              | 1                   | Experiment duration in minutes of trace to execute excluding warmup    |
| WarmupDuration               | int       | > 0                                                                                              | 0                   | Warmup duration in minutes(disabled if                                 |
| zero)                        |           |                                                                                                  |                     |                                                                        |
| IsPartiallyPanic             | bool      | true/false                                                                                       | false               | Pseudo-panic-mode only in Knative                                      |
| EnableZipkinTracing          | bool      | true/false                                                                                       | false               | Show loader span in Zipkin traces                                      |
| EnableMetricsScrapping       | bool      | true/false                                                                                       | false               | Scrap cluster-wide metrics                                             |
| MetricScrapingPeriodSeconds  | int       | > 0                                                                                              | 15                  | Period of Prometheus metrics scrapping                                 |
| GRPCConnectionTimeoutSeconds | int       | > 0                                                                                              | 60                  | Timeout for establishing a gRPC connection                             |
| GRPCFunctionTimeoutSeconds   | int       | > 0                                                                                              | 90                  | Maximum time given to function to                                      |
| execute[^4]                  |           |                                                                                                  |                     |                                                                        |

[^1]: The second granularity feature interprets each column of the trace as a second, rather than as a minute, and
generates IAT for each second. This feature is useful for fine-grained and precise invocation scheduling in experiments
involving stable low load.

[^2]: `_shift` modifies the IAT generation in the following way: by default, generation will create first invocation in
the beginning of the minute, with `_shift` modifier, it will be shifted inside the minute to remove the burst of
invocations from all the functions.

[^3]: Limits are set by resource->limits->CPU in the service YAML. `1vCPU` means limit of 1CPU is set, at the same time
execution is also limited by the container concurrency limit of 1. `GCP` means limits are set to multiples of 1/12th of
vCPU, based on the memory consumption of the function according to
this [table](https://cloud.google.com/functions/pricing#compute_time) for Google Cloud Functions.

[^4]: Function can execute for at most 15 minutes as in AWS
Lambda; https://aws.amazon.com/about-aws/whats-new/2018/10/aws-lambda-supports-functions-that-can-run-up-to-15-minutes/
