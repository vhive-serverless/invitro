# Loader

A load generator for rigorous scientific research on serverless computing based upon [faas-load-generator](https://github.com/eth-easl/faas-load-generator) and the example code of [vHive](https://github.com/ease-lab/vhive).

## Create an cluster

First, change the parameters (e.g., `GITHUB_TOKEN`) in the `script/setup.cfg` is necessary.

For creating a multi-node K8s cluster (pure containers), run the following.

```bash
bash ./scripts/setup/create_multinode_k8s.sh <master_node@IP> <worker_node@IP> ...
```

For creating a multi-node vHive cluster (firecracker uVMs), run the following.

```bash
bash ./scripts/setup/create_multinode_vhive.sh <master_node@IP> <worker_node@IP> ...
```

Run the following for a single-node setup.

```bash
bash create_singlenode_stock_k8s.sh <master_node@IP> 
```

**NB**: The multinode setting support 500 pods per node, whilst the single node only support 100 pods by default and need to manually stretch the limit if you want (yet to be automated).


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
bash scripts/experiments/run_trace_mode.sh <duration_in_minutes> <num_workers> <trace_path>

#* RPS mode
bash scripts/experiments/run_rps_mode.sh <start> <stop> <step> <duration_in_sec> <num_func> <wimpy|trace> <func_runtime> <func_mem> 
```

### Build the image for server function

```sh
$ make build <trace-func|busy-wait|sleep>
```
### Update gRPC protocol

```sh
$ make proto
```

---

For more details, please see the `Makefile`.
