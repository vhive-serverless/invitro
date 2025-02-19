# Deploy Info JSON

## The `deploy_info.json` file in the `yamls.tar.gz`  is used to identify the relative file paths for the Knative YAML manifests for deploying vSwarm functions. It is generated using `generate_deploy_info.py` Python script, also present under `yamls.tar.gz`, which outputs a JSON that embeds the yaml-location and pre-deployment commands for every vSwarm function.

## While the `deploy_info.json` file ships with the `yamls.tar.gz`, In order to regenerate the `deploy_info.json` run:
```console
tar -xzvf workloads/container/yamls.tar.gz -C workloads/container
cd workloads/container/yamls/
python3 generate_deploy_info.py
```