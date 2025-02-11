# Loader

A load generator for benchmarking serverless systems.

## Prerequisites

The experiments require a server-grade node running Linux (tested on Ubuntu 20, Intel Xeon). On CloudLab, one
can choose the APT cluster `d430` node.

## Create a cluster

### vHive cluster

First, configure `script/setup/setup.cfg`. You can specify there which vHive branch to use, loader branch, operation
mode (sandbox type), and maximum number of pods per node. All these configurations are mandatory. We currently support
the following modes: containerd (`container`), Firecracker (`firecracker`), and Firecracker with
snapshots (`firecracker_snapshots`).Loader will be cloned on every node specified as argument of the cluster create
script. The same holds for Kubernetes API server certificate.

* To create a multi-node cluster, specify the node addresses as the arguments and run the following command:

```bash
$ bash ./scripts/setup/create_multinode.sh <master_node@IP> <loader_node@IP> <worker_node@IP> ...
```

This command will create the following setup: control plane is placed on master node, loader node is used for running
loader and monitoring pods (mostly, Prometheus, if enabled in setup config), workers are used purely for working pods.
In
this setup, neither control plane nor workers are affected by loader and monitoring, creating more reliable measurements
of performance.

* Single-node cluster (experimental)

```bash
$ bash ./scripts/setup/create_singlenode_container.sh <node@IP>
```

This mode is only for debugging purposes, and there is no guarantees of isolation between the loader and the master-node
components.

### OpenWhisk cluster

See the instructions located in `openwhisk_setup/README.md`.

### Check cluster health (on the master node)

Once the setup scripts are finished, we need to check if they have completed their jobs fully, because, e.g., there
might be race conditions where a few nodes are unavailable.

* First, log out the status of the control plane components on the master node and monitoring deployments by running the
  following command:

```bash
$ bash ./scripts/util/log_kn_status.sh
```

* If you see everything is `Running`, check if the cluster capacity is stretched to the desired capacity by running the
  following script:

```bash
$ bash ./scripts/util/check_node_capacity.sh
```

If you want to check
the [pod CIDR range](https://www.ibm.com/docs/en/cloud-private/3.1.2?topic=networking-kubernetes-network-model), run the
following

```bash
$ bash ./scripts/util/get_pod_cidr.sh
```

* Next, try deploying a function (`myfunc`) with the desired `scale` (i.e., the number of instances that must be active
  at all times).

```bash
$ bash ./scripts/util/set_function_scale.sh <scale>
```

* One should verify that the system was able to start the requested number of function instances, by using the following
  command.

```bash
$ kubectl -n default get podautoscalers
```

## Tune the timing for the benchmark function

Before start any experiments, the timing of the benchmark function should be tuned so that it consumes the required
service time more precisely.

First, run the following command to deploy the timing benchmark that yields the number of execution units* the function
needs to run given a required service time.

* The execution unit is approximately 100 `SQRTSD` x86 instructions.

```bash
$ kubectl apply -f server/benchmark/timing.yaml
```

Then, monitor and collect the `ITERATIONS_MULTIPLIER` from the job logs as follows:

```bash
$ watch kubectl logs timing
```

Finally, set the `ITERATIONS_MULTIPLIER` in the function template `workloads/$SANDBOX_TYPE/trace_func_go.yaml` to the
value previously collected.

To account for difference in CPU performance set `ITERATIONS_MULTIPLIER=102` if using
Cloudlab `xl170` or `d430` machines. (Date of measurement: 18-Oct-2022)

## Executing vSwarm functions
If the input trace directory has a `mapper_output.json` file, which you would like to use as profiles for benchmark execution in the loader, run the following from the root of this directory:

```console
# install pre-requisites
sudo apt update
sudo apt -y install git-lfs pip xz-utils
git lfs install
git lfs fetch
git lfs checkout
```
This retrieves the `yamls.tar.gz` from Git LFS. Then, untar this tarball by running the following command from the root of this directory:

```bash
$ tar -xzvf workloads/container/yamls.tar.gz -C workloads/container/
```

## Single execution

To run load generator use the following command:

```bash
$ go run cmd/loader.go --config cmd/config_knative_trace.json
```
To run load generator to use vSwarm functions based on `mapper_output.json` run the following:

```bash
$ go run cmd/loader.go --config cmd/config_vswarm_trace.json
```
Additionally, one can specify log verbosity argument as `--verbosity [info, debug, trace]`. The default value is `info`.

To execute in a dry run mode without generating any load, set the `--dry-run` flag to `true`. This is useful for testing and validating configurations without executing actual requests.

For to configure the workload for load generator, please refer to `docs/configuration.md`.

There are a couple of constants that should not be exposed to the users. They can be examined and changed
in `pkg/common/constants.go`.

Sample sizes appropriate for performance evaluation vary depending on the platform. 
As a starting point for fine-tuning, we suggest at most 5 functions per core with SMT disabled. 
For example, 80 functions for a 16-core node. With larger sample sizes, trace replaying may lead to failures in function invocations.

## Build the image for a synthetic function

The reason for existence of Firecracker and container version is because of different ports for gRPC server. Firecracker
uses port 50051, whereas containers are accessible via port 80.

Synthetic function runs for a given time and allocates the required amount of memory:

* `trace-firecracker` - for Firecracker
* `trace-container` - for regular containers

For testing cold start performance:

* `empty-firecracker` - for Firecracker - returns immediately
* `empty-container` - for regular containers - returns immediately

```bash
$ make <trace-firecracker|trace-container|empty-firecracker|empty-container>
```

Pushing the images will require a write access to Github packages connected to this repository. Please refer to 
[this guide](https://docs.github.com/en/packages/working-with-a-github-packages-registry/working-with-the-container-registry#authenticating-with-a-personal-access-token-classic)
for authentication instructions.

## Clean up between runs

```bash
$ make clean
```

## Running the Experiment Driver

Within the tools/driver folder, Configure the driverConfig.json file based on your username on Cloudlab,
the address of the loader node that you are running the experiment on and any experiment parameters you'd like to set.
Note that you need to have the relevant trace files on the machine running the experiment driver, they will then get
transferred to the loader node.  
Then run the following command to launch an experiment:

```bash
$ go run experiment_driver.go -c driverConfig.json
```

For more details take a look at the README in the tools/driver folder.

---

For more options, please see the `Makefile`.

## Using OpenWhisk

For instructions on how to use the loader with OpenWhisk go to `openwhisk_setup/README.md`.

## Workflow Invocation
Generation of a Directed Acyclic Graph (DAG) workflow is supported by setting `"DAGMode: true"` in `cmd/config_knative_trace.json` (as specified in [`docs/configuration.md`](../docs/configuration.md)). 

Before invocation, DAGs will be iteratively generated based on the parameters: `width`,`depth`,`EnableDAGDataset`, until the remaining functions are insufficient to maintain the desired DAG structure. The remaining functions will be unused for the rest of the experiment. 

An example of the generated workflow can be seen here:

```bash
Functions available: 20
Width: 3
Depth: 4
EnableDAGDataset: false

DAG 1: f(0) -> f(1) -> f(3) -> f(5)
         \        
          \ -> f(2) -> f(4) -> f(6)
                        \
                         \ -> f(7)

DAG 2: f(8) -> f(9) -> f(12) -> f(15)
        \        
         \ -> f(10) -> f(13) -> f(16)
          \
           \-> f(11) -> f(14) -> f(17)
```
In the example, a single invocation of DAG 1 will result in 8 total functions invoked, with parallel invocations per branch. Invocation Frequency and IAT of DAGs 1 and 2 will depend on entry functions f(0) and f(8) respectively.

To obtain [reference traces](https://github.com/vhive-serverless/invitro/blob/main/docs/sampler.md#reference-traces) for DAG execution, use the following command:
```bash
git lfs pull 
tar -xzf data/traces/reference/sampled_150.tar.gz -C data/traces 
```
Microsoft has publicly released Microsoft Azure traces of function invocations from 10/18/2021 to 10/31/2021. From this trace, a [data sample](https://github.com/icanforce/Orion-OSDI22/blob/main/Public_Dataset) of DAG structures, representing the cumulative distribution of width and depth of DAGs during that period, was generated. Probabilities were applied to the data to derive the shape of the DAGs. The file `data/traces/example/dag_structure.csv` provides a simplified sample of the publicly released traces.

By default, the shape of the DAGs are automatically calculated at every iteration using the above mentioned cumulative distribution.
To manually set the shape of the DAGs, change the following parameters in `cmd/config_knative_trace.json`. Note that the number of functions in `TracePath` must be large enough to support the maximum size of the DAG. This ensures all DAGs generated will have the same width and depth.
```bash
"EnableDAGDataset": false,
"Width": <width>,
"Depth": <depth>
```

Lastly, start the experiment. This invokes all the generated DAGs with their respective frequencies.
```bash
go run cmd/loader.go --config cmd/config_knative_trace.json
```

## Running on Cloud Using Serverless Framework

**Currently supported vendors:** AWS

**Quick Setup for AWS Deployment:**
1. Install the dependencies required for AWS deployment
    ```bash
    bash ./scripts/setup/install_aws_dependencies.sh <loader_node@IP>`
    ```
2. In the loader node, set AWS credentials as environment variables
    ```bash
    export AWS_ACCESS_KEY_ID=
    export AWS_SECRET_ACCESS_KEY=
    export AWS_DEFAULT_REGION=us-east-1
    ```
   > For more fine-grained IAM setup, please refer to Serverless [setup guide](https://www.serverless.com/framework/docs/providers/aws/guide/credentials/)
3. In `cmd/config_knative_trace.json`, change `"Platform": "Knative"` to `"Platform": "AWSLambda"` (as specified in [`docs/configuration.md`](../docs/configuration.md))
    ```bash
    cd loader/
    sed -i 's/"Platform": "Knative"/"Platform": "AWSLambda"/g' cmd/config_knative_trace.json
    ```
4. Start the AWS deployment experiment:
    ```bash
    go run cmd/loader.go --config cmd/config_knative_trace.json
    ```
---
Note:
- Current deployment is via container image.
- Refer to [Single Execution](#single-execution) section for more details on the experiment configurations.
- **[Strongly Recommended]** For experiments with concurrency > 10, please raise a request to increase the default AWS Lambda concurrency limit of 10 to a higher value (e.g. 1000).
  - Go to the AWS Management Console, select the appropriate region (i.e. `us-east-1`) and search for `Service Quotas`
  - Under `Manage Quota`, select `AWS Lambda` service and click `View quotas` (Alternatively, click [here](https://us-east-1.console.aws.amazon.com/servicequotas/home/services/lambda/quotas))
  - Under `Quota name`, select `Concurrent executions` and click `Request increase at account level` (Alternatively, click [here](https://us-east-1.console.aws.amazon.com/servicequotas/home/services/lambda/quotas/L-B99A9384))
  - Under `Increase quota value`, input `1000` and click `Request`
  - Await AWS Support Team to approve the request. The request may take several days or weeks to be approved.