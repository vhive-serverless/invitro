# EasyLoader

A load generator for serverless computing based upon [faas-load-generator](https://github.com/eth-easl/faas-load-generator) and the example code of [vHive](https://github.com/ease-lab/vhive).

## Usage

For hotstart, run the following
```sh
$ make build
$ ./el --rps <request-per-sec> --duration <0-to-60-min> 
```

OR 

```sh
$ make ARGS="--rps 5 --duration 10" run
```

As for explicit cold start, replace `run` above with `coldstart`. 