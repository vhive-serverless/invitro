# Loader Quickstart Guide

This guide describes how to use the [Loader](https://github.com/eth-easl/loader) and [Sampler](https://github.com/eth-easl/sampler) to run a simple experiment running samples of various sizes on a serverless cluster.

## Installation

First, clone the [Loader][loader] and [Sampler][sampler] to your local machine.

```bash
git clone https://github.com/eth-easl/sampler.git
git clone https://github.com/eth-easl/loader.git
```

## Sampler

Next, we will use the sampler to obtain traces of various sizes.
The sampler contains multiple python scripts that can process traces. First, install the necessary dependencies.

```bash
cd sampler
pip install -r requirements.txt
```

We recommend using a virtual environment to avoid conflicts with other python packages.

We will start with the original Azure traces from [here](https://azurecloudpublicdataset2.blob.core.windows.net/azurepublicdatasetv2/azurefunctions_dataset2019/azurefunctions-dataset2019.tar.xz).

```bash
wget https://azurecloudpublicdataset2.blob.core.windows.net/azurepublicdatasetv2/azurefunctions_dataset2019/azurefunctions-dataset2019.tar.xz -P ./data/azure
tar -xf ./data/azure/azurefunctions-dataset2019.tar.xz -C data/azure/
```

This will download the traces and extract them to `./sampler/data/azure/` directory.

Because the original Azure traces have some inconsistencies, we must preprocess them before we can use them.

```bash
python -m sampler preprocess -t ./data/azure -o ./data/preprocessed -s 00:00:00 -dur 15
```

This will preprocess the traces and store them in `./sampler/data/preprocessed/` directory. The `-s` and `-dur` flags specify the start time and duration of the traces. The duration is specified in minutes, and the start time is in the format `dd:hh:mm`.

Now, we can use the sampler to obtain samples of various sizes. The In-vitro sampling method recursively samples traces from the original trace. We will start from our preprocessed traces and downsample with a step size of 1000.

```bash
python -m sampler sample -t ./data/preprocessed -o ./data/quickstart1000/ -min 1000 -max 44000 -st 1000
```

In the above command, the `-min` and `-max` flags specify the minimum and maximum sample size, and the `-st` flag specifies the step size. The `-o` flag specifies the output directory.
However, running a 1000 function sample will still require a lot of resources. Therefore, we will use the In-vitro sampling method again on the 1000 function sample to obtain even smaller samples. Because the In-vitro sampling method is recursive downsampling, the resulting samples will still be representative of the original trace.

```bash
python -m sampler sample -t ./data/quickstart1000/samples/1000 -o ./data/quickstart10/ -max 990 -min 10 -st 10
```

It is not necessary to use two different step sizes as above. We could have used a step size of 10 for the first sampling step. However, we wanted to show that the In-vitro sampling method can be used recursively to obtain samples of various sizes.

## Loader

### Setup

Next, we will use the [loader][loader] to run the samples we obtained on a serverless cluster.

```bash
cd loader
```

First, we will setup a serverless cluster. Any hardware setup to support [vHive][vhive] will work. Follow the [vHive quickstart guide](https://github.com/vhive-serverless/vHive/blob/main/docs/quickstart_guide.md#i-host-platform-requirements) to setup a cluster. We recommend renting nodes on [CloudLab](https://www.cloudlab.us/).

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
