# Loader

A load generator for benchmarking serverless systems.

## Pre-requisites

The experiments require a 2-socket server-grade node, running Linux (tested on Ubuntu 20, Intel Xeon). On CloudLab, one can choose the APT cluster `d430` node.

### Multi-node cluster

The master node should have at least two sockets, because, although we have isolated the loader with Cgroups, the isolation provided by our setup scripts is better achieved when it runs on one socket separate from the rest of the components running on master. 

### Single-node cluster

This mode is only for debugging purposes, and there is no guarantees of isolation between the loader and the master-node components.
## Create a cluster

First, change the parameters (e.g., `GITHUB_TOKEN`) in the `script/setup.cfg` is necessary.
Github token needs the `repo` and `admin:public_key` permissions.

* For creating a multi-node K8s cluster (pure containers) with maximum 500 pods per node, run the following.

```bash
$ bash ./scripts/setup/create_multinode_container_large.sh <master_node@IP> <worker_node@IP> ...
```

* For creating a multi-node K8s cluster (pure containers) with maximum 200 pods per node, run the following.

```bash
$ bash ./scripts/setup/create_multinode_container.sh <master_node@IP> <worker_node@IP> ...
```

* For creating a multi-node vHive cluster (firecracker uVMs), run the following.

```bash
$ bash ./scripts/setup/create_multinode_firecracker.sh <master_node@IP> <worker_node@IP> ...
```

* Run the following for a single-node setup. The capacity of this node is below 100 pods.

```bash
$ bash ./scripts/setup/create_singlenode_stock_k8s.sh <master_node@IP> 
```

### Check cluster health (on the master node)

Once the setup scripts are finished, we need to check if they have completed their jobs fully, because, e.g., there
might be race conditions where a few nodes are unavailable.

* First, log out the status of the control plane components on the master node and monitoring deployments by running the
  following command:

```bash
$ bash ./scripts/util/log_kn_status.sh
```

* If you see everything is `Running`, check if the cluster capacity sis stretched to the desired capacity by running the
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

Before start any experiments, the timing of the benchmark function should be tuned so that it consumes the required service time more precisely.

First, run the following command to deploy the timing benchmark that yields the number of execution units* the function needs to run given a required service time.

* The execution unit is approximately 100 `SQRTSD` x86 instructions.

```bash
$ kubectl apply -f server/benchmark/timing.yaml
```

Then, monitor and collect the `cold_iter_per_1ms` and `warm_iter_per_1ms` from the job logs as follows:

```bash
$ watch kubectl logs timing
```

Finally, set the `COLD_ITER_PER_1MS` and `WARM_ITER_PER_1MS` in the function
template `workloads/container/trace_func_go.yaml` based on `cold_iter_per_1ms` and `warm_iter_per_1ms` respectively.

To explain this further, `cold_iter_per_1ms` is for short executions (<1s), and `warm_iter_per_1ms` is for the longer
ones (>=1s).

To account for difference in CPU performance set `COLD_ITER_PER_1MS=102` and `WARM_ITER_PER_1MS=115` if you are using Cloudlab XL170 machines. (Date of measurement: 10-Aug-2022)

## Single execution

In the Trace mode, the loader replays the Azure trace.

For Trace mode, run the following command

```bash
cgexec -g cpuset,memory:loader-cg \
    make ARGS='-sample <sample_trace_size> -duration <minutes[1,1440]> -cluster <num_workers> -server <trace|busy|sleep> -tracePath <path_to_trace> -iatDistributionn <poission|uniform|equidistant> -warmup' run
```

In the RPS mode, the loader sweeps fixed number of invocations per second.

When using RPS mode, run the following command

```bash
cgexec -g cpuset,memory:loader-cg \
    make ARGS="-mode stress -start <initial_rps> -end <stop_rps> -step <rps_step> -slot <rps_step_in_seconds> -server <trace|busy|sleep> -totalFunctions <num_functions>" run 2>&1 | tee stress.log
```

NB: cgroups are for isolating the loader on master node from the control plane components.

## Experiment

For running experiments, use the wrapper scripts in the `scripts/experiments` directory.

```bash
#* Trace mode
$ bash scripts/experiments/run_trace_mode.sh \
    <duration_in_minutes> <num_workers> <trace_path> 

#* RPS mode
$ bash scripts/experiments/run_rps_mode.sh \
    <start> <stop> <step> <duration_in_sec> \
    <num_func> <wimpy|trace> <func_runtime> <func_mem> \
    <print-option: debug | info | all>
```

## Build the image for a synthetic function

A synthetic function runs for a given time and allocates the required amount of memory.

* `trace-func` mode counts iterations for fulfilling the service time.
* `busy-wait` mode uses timer-based spin-lock to consume service time.
* `sleep` mode is a noop that does nothing but idle waits.

```bash
$ make build <trace-func|busy-wait|sleep>
```

## Clean up between runs

```bash
$ make clean
```

---

For more options, please see the `Makefile`.

