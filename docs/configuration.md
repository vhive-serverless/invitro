# Loader configuration file format

| Parameter name               | Data type | Possible values                                                     | Default value       | Description                                                                     |
|------------------------------|-----------|---------------------------------------------------------------------|---------------------|---------------------------------------------------------------------------------|
| Seed                         | int64     | any                                                                 | 42                  | Seed for specification generator (for reproducibility)                          |
| Platform                     | string    | Knative, OpenWhisk                                                  | Knative             | The serverless platform the functions will be executed on                       |
| YAMLSelector                 | string    | wimpy, container, firecracker                                       | container           | Service YAML depending on sandbox type                                          |
| EndpointPort                 | int       | > 0                                                                 | 80                  | Port to be appended to the service URL                                          |
| TracePath                    | string    | string                                                              | data/traces         | Folder with Azure trace dimensions (invocations.csv, durations.csv, memory.csv) |
| Granularity                  | string    | minute, second                                                      | minute              | Granularity for trace interpretation[^1]                                        |
| OutputPathPrefix             | string    | any                                                                 | data/out/experiment | Results file(s) output path prefix                                              |
| IATDistribution              | string    | exponential, exponential_shift, uniform, uniform_shift, equidistant | exponential         | IAT distribution[^2]                                                            |
| ExperimentDuration           | int       | > 0                                                                 | 1                   | Experiment duration in minutes of trace to execute excluding warmup             |
| WarmupDuration               | int       | > 0                                                                 | 0                   | Warmup duration in minutes(disabled if zero)                                    |
| IsPartiallyPanic             | bool      | true/false                                                          | false               | Pseudo-panic-mode only in Knative                                               |
| EnableZipkinTracing          | bool      | true/false                                                          | false               | Show loader span in Zipkin traces                                               |
| EnableMetricsScrapping       | bool      | true/false                                                          | false               | Scrap cluster-wide metrics                                                      |
| MetricScrapingPeriodSeconds  | int       | > 0                                                                 | 15                  | Period of Prometheus metrics scrapping                                          |
| GRPCConnectionTimeoutSeconds | int       | > 0                                                                 | 60                  | Timeout for establishing a gRPC connection                                      |
| GRPCFunctionTimeoutSeconds   | int       | > 0                                                                 | 90                  | Maximum time given to function to execute[^3]                                   |

[^1]: The second granularity feature interprets each column of the trace as a second, rather than as a minute, and
generates IAT for each second. This feature is useful for fine-grained and precise invocation scheduling in experiments
involving stable low load.

[^2]: `_shift` modifies the IAT generation in the following way: by default, generation will create first invocation in the beginning of the minute, with `_shift` modifier, it will be shifted inside the minute to remove the burst of invocations from all the functions.

[^3]: Function can execute for at most 15 minutes as in AWS
Lambda; https://aws.amazon.com/about-aws/whats-new/2018/10/aws-lambda-supports-functions-that-can-run-up-to-15-minutes/
