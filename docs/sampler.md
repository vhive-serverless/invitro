# Trace Sampler

## Setup

```console
# install pre-requisites
sudo apt update
sudo apt -y install git-lfs pip xz-utils
git lfs install

# clone this repo to ./sampler
cd sampler
git lfs fetch
git lfs checkout
pip install -r requirements.txt
```

## Pre-processing the original trace (mandatory)

### Description

The pre-processing logic works in 3 phases, first two of which perform cleaning and transforming of the original trace,
whereas the third phase produces an excerpt of the clean trace, according to the user-defined start and end time.

The original trace files (invocations, durations, memory) may contain different applications and functions (i.e., their
hashes).
Hence, the first step is to derive a set of rows for each file with functions and applications that appear in all files.
Since the original memory spec file contains rows per application and not per function, the second step is
transforming the file to contain per-function rows: all memory characteristics of an application
in the original trace are divided evenly among the rows, each of which corresponds to one of the application's
functions.

The third preprocessing step is producing a trace excerpt with the user-defined start time and
duration of the cleaned trace.

### Workflow

First, download the original trace files
from [here](https://azurecloudpublicdataset2.blob.core.windows.net/azurepublicdatasetv2/azurefunctions_dataset2019/azurefunctions-dataset2019.tar.xz)
and extract the CSV files (default location: `data/azure/`).

```console
wget https://azurecloudpublicdataset2.blob.core.windows.net/azurepublicdatasetv2/azurefunctions_dataset2019/azurefunctions-dataset2019.tar.xz -P ./data/azure
tar -xf ./data/azure/azurefunctions-dataset2019.tar.xz -C data/azure/
```

Then, run the following command to preprocess the trace.

```console
python3 -m sampler preprocess -h

usage:  preprocess [-h] [-t path] -o path -s start -dur duration

optional arguments:
  -h, --help            show this help message and exit
  -t path, --trace path
                        Path to the Azure trace files
  -o path, --output path
                        Output path for the preprocessed traces
  -s start, --start start
                        Time in dd:hh:mm format, at which the excerpt of the postprocessed trace should begin, first day is day 0
  -dur duration, --duration duration
                        Duration in minutes of the excerpt extracted from the postprocessed trace             
```

## Sampling

### Description

The sampling algorithm requires a clean, i.e., preprocessed, trace, which it derives samples.
The user specifies the size range for samples to be generated, specifying the minimum and maximum sizes
as well as the step of the sweep. The user also specifies the number of random sampling trials for each sample size.
For each trial, the sample is evaluated by computing its Wasserstein distance (WD) from the original trace -- for each
minute
in the trace -- in two dimensions, namely the number of invocations in a minute and the amount of CPU and memory
resources
used by the functions. The latter is estimated (at the minute granularity for each function, which then are summed up)
by multiplying the number of invocations by the average (mean) duration and by the memory footprint.

Note that the latter (resource) WD metric is normalized by 1000 000 to be in the same ballpark as the former (
invocation) WD metric. The latter metric is found empirically to be 1000 000 higher than the former because function
invocation duration average is 1000ms, invocation count average is <10/min, and memory average is 200MB.

First, the sampler derives the largest sample, for which it makes several attempts (trials) when choosing the sample
with the smallest aggregate WD distance (mean of the invocation and resource WDs). Then, each smaller sample is derived
from the previous, larger sample; hence guaranteeing that large samples always include smaller samples that guarantees
monotonic load increase (in terms of resource usage) when sweeping the sample size.

### Workflow

```console
python3 -m sampler sample -h

usage: sample [-h] -t path -o path [-min integer] [-st integer] [-max integer] [-tr integer]

optional arguments:
  -h, --help            show this help message and exit
  -t path, --source_trace path
                        Path to trace to draw samples from
  -orig path, --original_trace path
                        Path to the Azure (or other original) trace files, required to maximize the derived sample's representativity (WD from the original trace)
  -o path, --output path
                        Output path for the resulting samples
  -min integer, --min-size integer
                        Minimum sample size (#functions).
  -st integer, --step-size integer
                        Step (#functions) in sample size during sampling.
  -max integer, --max-size integer
                        Maximum sample size (#functions).
  -tr integer, --trial-num integer
                        Number of sampling trials for each sample size.
```

## Reference traces

The reference traces are stored in `data/traces/reference` folder of this repository, as `preprocessed.tar.gz` and
`sampled.tar.gz` files stored in Git LFS.

`preprocessed.tar.gz` contains the preprocessed traces for the original Azure trace for day 1, 09:00:00-11:30:00 (150
minutes total).

`sampled.tar.gz` contains the sampled traces for preprocessed trace from `preprocessed.tar.gz`. Sample sizes are 50-3k
functions with step 50 and 3k-24k with step 1k.

The reference traces were obtained by running the following commands:

```console
python3 -m preprocess  -t data/azure/ -o data/reference/preprocessed_150 -s 00:09:00 -dur 150

python3 -m sample -t data/reference/preprocessed_150 -o data/reference/sampled_150 -min 3000 -st 1000 -max 24000 -tr 16
python3 -m sample -t data/reference/sampled_150/samples/3000 -o data/reference/sampled_150 -min 50 -st 50 -max 3000 -tr 16
```

## Tools

### Plotting

**Note:** Currently plotting is broken and has been disabled. (issue filled)

```console
python3 -m sampler plot -h

usage: sampler plot [-h] -t path -k [invocation, runtime, memory] -s path

optional arguments:
  -t path, --trace path
                        Path to the trace
  -k [invocation, runtime, memory], --kind [invocation, runtime, memory]
                        Generate CDF for a single dimension
  -s path, --sample path
                        Path to the sample to be visualised
  -o path, --output path
                        Output path for the produced figures
```

### Timeline analysis

Tools that can be used to generate the timeline of a trace are available in the `tools` directory.

- [`generateTimeline/generateTimeline.go`](/tools/generateTimeline/generateTimeline.go) - generates a timeline of the
  trace in either milliseconds or minute scale.
- [`plotTimeline/plotting.py`](/tools/plotTimeline/plotting.py) - contains scripts used to plot various trace
  characteristics

A jupyter notebook containing examples of the timeline analysis is available
in [`plotTimeline/analysis.ipynb`](/tools/plotTimeline/analysis.ipynb)
