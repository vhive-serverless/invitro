# Loader Experiment Quickstart

This guide describes how to use the [Loader](https://github.com/eth-easl/loader) and [Sampler](https://github.com/eth-easl/sampler) to run a simple experiment running samples of various sizes on a serverless cluster.

## Installation

First, clone the [Loader][loader] and [Sampler][sampler] to your local machine.

```bash
git clone https://github.com/eth-easl/sampler.git
git clone https://github.com/eth-easl/loader.git
```

Follow the guides in [sampler][sampler] to obtain downsampled samples of various sizes.

### Setup

We will use the [loader][loader] to run the samples we obtained on a serverless cluster.

```bash
cd loader
```

First, we will setup a serverless cluster. Any hardware setup to support [vHive][vhive] will work. Follow the [vHive quickstart guide](https://github.com/vhive-serverless/vHive/blob/main/docs/quickstart_guide.md#i-host-platform-requirements) to setup a cluster. We recommend renting nodes on [CloudLab](https://www.cloudlab.us/), `d430` and `xl170` are the recommended node types that have been tested.
Instead of manually installing [vHive][vhive] and the [Loader][loader] on the cluster, the [Loader][loader] contains a setup script that will automatically install [vHive][vhive] and the [Loader][loader] on the cluster.

Configure the `scripts/setup/setup.cfg` file to specify the cluster configuration.

```cfg
VHIVE_BRANCH='hy'
LOADER_BRANCH='main'
CLUSTER_MODE='container' # choose from {container, firecracker, firecracker_snapshots}
PODS_PER_NODE=240
GITHUB_TOKEN=$HOME/.git_token_loader
```

- Because the [Loader][loader] is currently a private project, it requires a github token to install on the cluster. The token needs `repo` and `admin:public_key` permissions.

We can now use the included setup scripts to install [vHive][vhive] and the [Loader][loader] on the cluster.

```bash
bash ./scripts/setup/create_multinode.sh <master_node@IP> <worker_node@IP> ...
```

### Running an experiment

Once the setup scripts are finished, we can proceed to running an experiment. We will be using the experiment driver included with the loader to run our experiment.
Within the `tools/driver` directory, we will configure the `driverConfig.json` file to our specifications. The following is an example configuration file.

```json
{
  "username": "your_username",
  "experimentName": "quickstart",
  "localTracePath": "your_username/sampler/data/quickstart10/samples/",
  "loaderTracePath": "loader/data/traces",
  "loaderAddresses": ["pc784.emulab.net"],
  "beginningFuncNum": 10,
  "stepSizeFunc": 10,
  "maxFuncNum": 100,
  "experimentDuration": 10,
  "warmupDuration": 2,
  "workerNodeNum": 1,
  "outputDir": "measurements",
  "YAMLSelector": "container",
  "IATDistribution": "exponential",
  "loaderOutputPath": "data/out/experiment",
  "partiallyPanic": false,
  "EnableZipkinTracing": false,
  "EnableMetricsScrapping": true,
  "MetricScrapingPeriodSeconds": 60,
  "separateIATGeneration": false
}
```

Notice that we configure the `localTracePath` to the samples we generated in the sampler. By configuring the driver to start with 10 functions and increase the sample size by 10, we will run the experiment on samples of size 10, 20, 30, ..., 100.
Now, we can run the experiment.

```bash
go run experiment_driver.go -c driverConfig.json
```

The experiment will run on the cluster and the results will be stored in the `measurements` directory.

[loader]: https://github.com/eth-easl/loader
[sampler]: https://github.com/eth-easl/sampler
[vhive]: https://github.com/vhive-serverless/vHive
