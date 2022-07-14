# Loader

A load generator for benchmarking serverless systems based upon [faas-load-generator](https://github.com/eth-easl/faas-load-generator) and the example code in [vHive](https://github.com/ease-lab/vhive).

## Set up a cluster

First, change the parameters (e.g., `GITHUB_TOKEN`) in the `script/setup.cfg` is necessary.

On cloudlab, select the **Utah cluster APT `d430`.**

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

### Check cluster health

* First, log out the status of the control plane and monitoring deployments by running the following command:

```bash
$ bash ./scripts/util/log_kn_status.sh
```

* If you see everything is `Running`, check if the cluster pod limit is stretched to the desired capacity by running the following script:

```bash
$ bash ./scripts/util/check_node_capacity.sh
```

(if you want to check the pod CIDR range, run the following)

```bash
$ bash ./scripts/util/get_pod_cidr.sh
```

* Next, try deploying a function (`myfunc`) with the desired `scale`.

```bash
$ bash ./scripts/util/scale_pod.sh <scale>
```

* Then, if the function can scale properly using the following command, then you are good to go.

```bash
$ kubectl -n default get podautoscalers
```


## Tune the benchmark function

Before start any experiments, you should tune the benchmark function. 

First, run the following command to deploy the timing benchmark.

```bash
$ kubectl apply -f server/benchmark/timing.yaml
```

Then, monitor and collect the `cold_iter_per_1ms` and `warm_iter_per_1ms` from the job logs as follows:
```bash
$ watch kubectl logs timing
```
Finally, set the `COLD_ITER_PER_1MS` and `WARM_ITER_PER_1MS` in the function template `workloads/container/trace_func_go.yaml` based on `cold_iter_per_1ms` and `warm_iter_per_1ms` respectively.



## Single execution

For Trace mode, run the following command

```bash
cgexec -g cpuset,memory:loader-cg \
    make ARGS='-sample <sample_trace_size> -duration <minutes[1,1440]> -cluster <num_workers> -server <trace|busy|sleep> -tracePath <path_to_trace> -warmup' run
```

When using RPS mode, run the following command

```bash
cgexec -g cpuset,memory:loader-cg \
    make ARGS="-mode stress -start <initial_rps> -end <stop_rps> -step <rps_step> -slot <rps_step_in_seconds> -server <trace|busy|sleep> -totalFunctions <num_functions>" run 2>&1 | tee stress.log
```

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

## Build the image for server functions

```bash
$ make build <trace-func|busy-wait|sleep>
```
## Update gRPC protocol

```bash
$ make proto
```

## Clean up between runs

```bash
$ make clean
```

---

For more options, please see the `Makefile`.
