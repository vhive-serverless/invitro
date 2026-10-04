## Context
### TODO
Ensure all 4 traces can be ran
- ~~Downloading of dataset~~
  - ~~Rclone~~
  - ~~Compressed good middle ground~~
- ~~Sampling~~
- Loading
  - Perform runs ensure Loader no issue.
- Run Analysis
  - Look at information available

After that, look at information available from each run
- Information available

## Node Setup
### Renting CloudLab Nodes
Tried the vHive profile with 1 node. (Emulab d430)
[Renting with vHive Profile](https://www.cloudlab.us/p/ntu-cloud/vhive-ubuntu24)

### Connecting to CloudLab Nodes
Connect to node
`ssh -o ServerAliveInterval=60 -A <user@url>`
`ssh -o ServerAliveInterval=60 -A bryanFB@pc841.emulab.net` (node-000)

Check if node detects ssh-agent
`echo "$SSH_AUTH_SOCK"`

### Setup Main Branch
Clone latest branch
`git clone --branch main https://github.com/vhive-serverless/invitro.git`
`git clone --branch fyp_report https://github.com/vhive-serverless/invitro.git`

Run single_node_installer
`cd invitro/`
`bash ./scripts/setup/create_singlenode_container.sh <user@url>`
`bash ./scripts/setup/create_singlenode_container.sh bryanFB@pc841.emulab.net`

### UV Install
`curl -LsSf https://astral.sh/uv/install.sh | sh`
`source $HOME/.local/bin/env`
`uv init`
`uv add -r requirements.txt`
`uv sync`
`source .venv/bin/activate`

### Git Setup
`git config --local user.name "16fb"`
`git config --local user.email "wongwenpingbryan@gmail.com"`

## Downloading Original Datasets To System
### RClone + Google Cloud
Copy over essential configs (In PowerShell)
`scp -r "C:\Users\toomu\Desktop\Projects\Projects\Actual_Invitro_Development\remote_setup" bryanFB@pc841.emulab.net:~/invitro/`

Install
`sudo apt install rclone`

Find location of local config file + make folder
`rclone config file`

Save rclone.conf to that file location
`cp ~/invitro/remote_setup/rclone.conf ~/.config/rclone/rclone.conf`

Test, list directories in top level of your drive
`rclone lsd remote:`

Download data to local
`rclone copy remote:RClone/FYP_full ~/invitro/data --ignore-existing --progress`
`rclone copy remote:RClone/FYP_compressed ~/invitro/data --ignore-existing --progress`
Times Test
- Full -> 45 min upload, 21 min download
- Compressed -> Quite awhile upload still, 4 min download

## Generate Samples (Locally)
2 hr trace from each trace-type. (normally 1 hour worth of trace is ran)

### Local python
`uv sync`
`deactivate`
`.venv\Scripts\activate`

### Azure2019
Preprocess and select 120 mins starting from 09:00
``` Bash
python -m sampler preprocess -t data/datasets/azure2019/original -o data/datasets/azure2019/preprocessed120 -s 00:09:00 -dur 120
```

Subsample trace
``` Bash
python -m sampler sample -t data/datasets/azure2019/preprocessed120 -orig data/datasets/azure2019/preprocessed120 -o data/datasets/azure2019/sampled_120 -min 3000 -st 1000 -max 24000 -tr 16
python -m sampler sample -t data/datasets/azure2019/sampled_120/samples/3000 -orig data/datasets/azure2019/preprocessed120 -o data/datasets/azure2019/sampled_120_2 -min 200 -st 50 -max 3000 -tr 16
python -m sampler sample -t data/datasets/azure2019/sampled_120_2/samples/200 -orig data/datasets/azure2019/preprocessed120 -o data/datasets/azure2019/sampled_120_3 -min 10 -st 10 -max 200 -tr 16
```

Information about trace
- Preprocessed Trace
  - 24123 rows (functions)
- Samples
  - 3000 to 24000 (increments of 1000)
  - 200 to 3000 (increments of 50)
  - 10 to 200 (increments of 10)

### Azure2021
``` bash
python -m sampler preprocess2021 -t data/datasets/azure2021/AzureFunctionsInvocationTraceForTwoWeeksJan2021.txt -o data/datasets/azure2021/preprocessed_120 -s 00:09:00 -dur 120 -thresh 100

python -m sampler sample -t data/datasets/azure2021/preprocessed_120 -orig data/datasets/azure2021/preprocessed_120 -o data/datasets/azure2021/sampled_120 -min 10 -st 5 -max 38 -tr 16

python -m sampler filter2021 -t data/datasets/azure2021/AzureFunctionsInvocationTraceForTwoWeeksJan2021.txt -st data/datasets/azure2021/sampled_120/samples/35 -o data/datasets/azure2021/filtered_120_35 -s 00:09:00 -dur 120
```

Information about trace (Likely can just put whole trace in)
- Preprocessed Trace
  - 6050 invocations
  - 38 functions
- Intermedites Samples
  - 10 to 38 (increments of 5)
- Final Sample
  - 6013 invocations
  - 35 functions

### Huawei2023
``` bash
# Preprocess
`python -m sampler preprocessHuawei2023 -t data/datasets/huawei2023/private_dataset -o data/datasets/huawei2023/preprocessed_120 -s 00:09:00 -dur 120`
# Sample
`python -m sampler sample -t data/datasets/huawei2023/preprocessed_120 -orig data/datasets/huawei2023/preprocessed_120 -o data/datasets/huawei2023/sampled_120 -min 20 -st 10 -max 100 -tr 16 -res 1000000`
```

Information about trace
- Preprocessed
  - 100 functions
- Sampled
  - 20 to 100 (increments of 10)

### IBM2026
``` bash
# REMOVE THE DEDUPLICATION CODE IN PREPROCESSIBM2026

# Preprocess to Azure2021
python -m sampler preprocessIBM2026 -t data/datasets/ibm2026/pickle_data -o data/datasets/ibm2026/converted_120 -s 00:09:00 -dur 120

# Sample
python -m sampler preprocess2021 -t data/datasets/ibm2026/converted_120/IBM2026AsAzure2021.csv -o data/datasets/ibm2026/preprocessed_120 -s 00:00:00 -dur 120 -thresh 100

python -m sampler sample -t data/datasets/ibm2026/preprocessed_120 -orig data/datasets/ibm2026/preprocessed_120 -o data/datasets/ibm2026/sampled_120 -min 100 -st 20 -max 500 -tr 16

python -m sampler filter2021 -t data/datasets/ibm2026/converted_120/IBM2026AsAzure2021.csv -st data/datasets/ibm2026/sampled_120/samples/480 -o data/datasets/ibm2026/filtered_120 -s 00:00:00 -dur 120
```

Information about trace
- Converted Trace
- Preprocessed Trace
  - 3049100 invocations
  - 521 functions
- Filtered Trace
  - 2899902 invocations/rows
  - 480 functions

ISSUE
- Sampler in azure2021 sample removes duplicate rows automatically.
- Drops 62482 duplicate rows.
- Deduplicated removed from code, and samples procured.

## Test that traces can be executed
Each experiment duration runs for 4 minutes

Install Go
`sudo apt-get install -y golang`

Extra CLI Arguments
`--generated=true`
`--iatGeneration=true`
`--dryRun=true`
`--verbosity=info` `--verbosity=debug` `--verbosity=trace`

Example Dataset
`$ go run cmd/loader.go --config cmd/config_knative_trace.json --verbosity=debug`

Azure2019
`$ go run cmd/loader.go --config cmd/config_fyp_azure2019.json --verbosity=debug`

IBM2026 (ok!)
`$ go run cmd/loader.go --config cmd/config_fyp_ibm2026.json --verbosity=debug`

Azure2021 (ok!)
`$ go run cmd/loader.go --config cmd/config_fyp_azure2021.json --verbosity=debug`

Huawei2023 (ok!)
`$ go run cmd/loader.go --config cmd/config_fyp_huawei2023.json --verbosity=debug`

# Perfrom Experiments

## UV Install
`curl -LsSf https://astral.sh/uv/install.sh | sh`
`source $HOME/.local/bin/env`
`uv init`
`uv add -r requirements.txt`
`uv sync`
`source .venv/bin/activate`