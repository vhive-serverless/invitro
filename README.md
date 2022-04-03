# Loader

A load generator for serverless computing based upon [faas-load-generator](https://github.com/eth-easl/faas-load-generator) and the example code of [vHive](https://github.com/ease-lab/vhive).

## Usage

For more details, please see `Makefile`.
### Execution

For Trace mode, run the following command

```bash
cgexec -g cpuset,memory:loader-cg \
    make ARGS='-sample <trace size> -duration <1-1440 in minutes> -cluster <worker nodes> -server <function server> -warmup' run
```

When using RPS mode, run the following command

```bash
cgexec -g cpuset,memory:loader-cg \
    make ARGS="-mode stress -start <initial RPS> -step <step size> -slot <step duration in seconds> -server <function server> " run 2>&1 | tee stress.log
```

### Build the image for server function

```sh
$ make build <trace-func|busy-wait|sleep>
```
### Update gRPC protocol

```sh
$ make proto
```

### Run experiments 

Scripts under the `\scripts\experiments\` directory have more configurations.