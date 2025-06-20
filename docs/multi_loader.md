# Multi-Loader

A wrapper around loader to run multiple experiments in sequence with additional features like validation, dry-run, log collection

## Prerequisites
As a wrapper around loader, multi-loader requires the initial cluster setup to be completed. See [vHive Loader to create a cluster](https://github.com/vhive-serverless/invitro/blob/main/docs/loader.md#create-a-cluster)

## Configuration
### Multi-Loader Configuration
| Parameter name      | Data type          | Possible values | Default value | Description                                                |
|---------------------|--------------------|-----------------|---------------|------------------------------------------------------------|
| Studies         | []LoaderStudy | N/A             | N/A           | A list of loader studies with their respective configurations. See [LoaderStudy](#loaderstudy) |
| BaseConfigPath      | string             | "tools/multi_loader/base_loader_config.json" | N/A           | Path to the base configuration file                         |
| IatGeneration         | bool                   | true, false                   | false         | (Optional) Whether to Generate iats only and skip invocations |
| Generated             | bool                   | true, false                   | false         | (Optional) if iats were already generated         |
| PreScript           | string             | any bash command | ""           | (Optional) A global script that runs once before all experiments |
| PostScript          | string             | any bash command | ""           | (Optional) A global script that runs once after all experiments  |
| MasterNode          | string             | "10.0.1.1"      | ""           | (Optional) The node acting as the master                    |
| AutoScalerNode      | string             | "10.0.1.1"      | ""           | (Optional) The node responsible for autoscaling             |
| ActivatorNode       | string             | "10.0.1.1"      | ""           | (Optional) The node responsible for activating services     |
| LoaderNode          | string             | "10.0.1.2"      | ""           | (Optional) The node responsible for running the loaders     |
| WorkerNodes         | []string           | ["10.0.1.3"]    | []           | (Optional) A list of worker nodes to distribute the workload|
| Metrics             | []string           | ["activator", "autoscaler", "top", "prometheus"] | []    | (Optional) List of supported metrics that the multi-loader will collate at the end of each experiment

> **_Note_**: 
> Node addresses are optional as Multi-Loader uses `kubectl` to find them. If needed, you can define addresses manually, which will override the automatic detection.

#### **Metrics Collected**  
More information regarding the metrics that can be collected at the end of each experiment:

- **activator** – Captures Knative Activator logs which includes health and readiness status updates for service endpoints it manages
- **autoscaler** – Tracks Knative autoscaler decisions, scaling events, and resource utilization
- **top** – Provides CPU and memory usage statistics for each nodes
- **prometheus** – Captures a snapshot of Prometheus’s TSDB, which includes system-wide performance metrics. The snapshot can be restored in an active Prometheus instance for further analysis. For details, see the *Restore Prometheus Data* section in [this guide](https://devopstales.github.io/kubernetes/backup-and-retore-prometheus/).


### LoaderStudy
| Parameter name        | Data type              | Possible values               | Default value | Description                                                        |
|-----------------------|------------------------|-------------------------------|---------------|--------------------------------------------------------------------|
| Config                | map[string]interface{} | Any field in [LoaderConfiguration](https://github.com/vhive-serverless/invitro/blob/main/docs/configuration.md#loader-configuration-file-format) except `Platform`| N/A           | The configuration for each loader experiment which overrides configurations in baseLoaderConfig                      |
| Name                  | string                 | N/A                           | N/A           | The name of the loader experiment                                  |
| TracesDir             | string                 | N/A                           | N/A           | Directory containing the traces for the experiment                 |
| TracesFormat          | string                 | "data/traces/example_{}"      | N/A           | Format of the trace files **The format string "{}" is required** |
| TraceValues           | []interface{}          | ["any", 0, 1.1]               | N/A           | Values of the trace files Replaces the "{}" in TraceFormat             |
| OutputDir             | string                 | any                           | data/out/{Name} | (Optional) Output directory for experiment results                 |
| Verbosity             | string                 | "info", "debug", "trace"      | "info"        | (Optional) Verbosity level for logging the experiment             |
| PreScript             | string                 | any bash Command              | ""           | (Optional) Local script that runs this specific experiment |
| PostScript            | string                 | any bash Command              | ""           | (Optional) Local script that runs this specific experiment |
| Sweep                 | [][SweepOptions](#sweepoptions)        | N/A                           | N/A           | (Optional) List of sweep options for the experiment |
| SweepType             | string                 | "linear", "grid"              | "grid"        | (Optional) Determines how the sweep options are applied: `grid` means all permutations, while `linear` pairs corresponding elements from sweep options, similar to Python's zip() function |

> **_Important_**: 
>
> Only one of the following is required:
> 1. `TracesDir`, or
> 2. `TracesFormat` and `TraceValues`, or
> 3. `TracePath` within the `LoaderExperiment`'s `Config` field
>
> If more than one is defined, the order of precedence is as follows:  
> 1. `TracesDir`,  
> 2. `TracesFormat` and `TraceValues`,  
> 3. `TracePath`
>
> The `Platform` field must not be overridden and should only be defined in the base config.
>
> The `IatGeneration` and `Generated` fields may not function as expected when handling multiple experiments due to limitations in the loader implementation.

> **_Note_**: 
>
> The `Config` field follows the same structure as the [LoaderConfiguration](https://github.com/vhive-serverless/invitro/blob/main/docs/configuration.md#loader-configuration-file-format). 
> Any field defined in `Config` will override the corresponding value from the configuration in `BaseConfigPath`, but only for that specific experiment. 
> For example, if `BaseConfigPath` has `ExperimentDuration` set to 5 minutes, and you define `ExperimentDuration` as 10 minutes in `Config`, that particular experiment will run for 10 minutes instead.

### SweepOptions

| Parameter name | Data type     | Possible values               | Default value | Description                                                                |
|----------------|---------------|-------------------------------|---------------|----------------------------------------------------------------------------|
| Field          | string        | Any valid field name          | N/A           | Any field in [LoaderConfiguration](https://github.com/vhive-serverless/invitro/blob/main/docs/configuration.md#loader-configuration-file-format), `PreScript` and `PostScript`. Cannot be `TracePath`, `OutputDir` or `Platform` as they are reserved fields |
| Values         | []interface{} | Any valid value type          | N/A           | List of values for the sweep options. These will be used as the sweep values for the experiment |
| Format         | string        | Any string containing `{}`    | N/A           | Optional format string to customize the sweep. The `{}` placeholder should be included to replace with sweep values |

#### Example:
  - `Field` should not be empty.
  - Reserved fields like `TracePath` and `OutputDir` cannot be used.
  - `Values` must contain at least one item.
  - If `Format` is provided, it must include the placeholder `{}`.
  
  ```json
  {
    "Field": "ExperimentDuration",
    "Values": [10, 20],
  },
  {
    "Field": "Prescript",
    "Values": ["example1", "example2", "example3"],
    "Format": "command_{}_to_run"
  }
  ```

> **_Note_**: 
> You can use PreScript and PostScript as sweep options; however, these scripts will only execute at the local level. This means that the global PreScript and PostScript will not be overridden by the sweep-specific scripts.



## Command Flags

The multi-loader accepts the following command-line flags. 

> **_Note_**: These flags will subsequently be used during the execution of loader.go for **<u>every experiment</u>**. If you would like to define these flag for specific experiments only, define it in [LoaderStudy](#loaderstudy)

Available flags:

- **`--multiLoaderConfig`** *(default: `tools/multi_loader/multi_loader_config.json`)*:  
  Specifies the path to the multi-loader configuration file. This file contains settings and parameters that define how the multi-loader operates [see above](#multi-loader-configuration)

- **`--verbosity`** *(default: `info`)*:  
    Sets the logging verbosity level. You can choose from the following options:
    - `info`: Standard information messages.
    - `debug`: Detailed debugging messages.
    - `trace`: Extremely verbose logging, including detailed execution traces.

- **`--failFast`** *(default: `false`)*:  
  Determines whether the multi-loader should skip the study immediately after a failure. By default, the loader retries a failed experiment once with debug verbosity and skips the study only if the second attempt also fails. Setting this flag to `true` prevents the retry and skips the study after the first failure.



## Multi-loader Overall Flow

1. **Initialization**
    - Flags for configuration file path, verbosity, IAT generation, and execution mode are parsed
    - Logger is initialized based on verbosity level

3. **Experiment Execution Flow**
    - The multi-loader runner is instantiated with the provided configuration path.
    - A dry run is executed to validate the setup for all studies:
        - If any dry run fails, the execution terminates.
    - If all dry runs succeed, proceed to actual runs:
        - Global pre-scripts are executed.
        - Each experiment undergoes the following steps:
            1. **Pre-Execution Setup**
                - Experiment-specific pre-scripts are executed.
                - Necessary directories and folders are created.
                - Each sub-experiment is unpacked and prepared
            2. **Experiment Invocation**
                - The loader is executed with generated configurations and related flags
            3. **Post-Execution Steps**
                - Experiment-specific post-scripts are executed
                - Cleanup tasks are performed
  
4. **Completion**
    - Global post-scripts are executed.
    - Run Make Clean

### How the Dry Run Works

The dry run mode executes the loader with the `--dryRun` flag set to true after the unpacking of experiments defined in the multi-loader configurations.

In this mode, the loader performs the following actions:

- **Configuration Validation**: It verifies the experiment configurations without deploying any functions or generating invocations.
- **Error Handling**: If a fatal error occurs at any point, the experiment will halt immediately.

The purpose is to ensure that your configurations are correct and to identify any potential issues before actual execution.