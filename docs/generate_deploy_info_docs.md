# Deploy Info JSON

The `deploy_info.json` file is used to identify deployment information for services and their pre-dependencies. It is pre-generated, and stored inside the `vSwarm_deploy_metadata.tar.gz` to contain deployment information for vSwarm functions.

## Schema and Usage

The `deploy_info.json` file in the `vSwarm_deploy_metadata.tar.gz`  is used to identify the relative file paths for the Knative YAML manifests for deploying vSwarm functions. It also contains the path of YAML files needed as part of the pre-deployment commands to run certain vSwarm benchmarks, for example the `online-shop-database` which requires to be deployed before running `cartservice` benchmark.

The `deploy_info.json` has the following schema:
```console
{
    vswarm-function-name:
        {
            YamlLocation: /path/to/yaml
            PredeploymentPath: [/path/to/predeployment-database/yaml]
        }
}
```

The `PredeploymentPath` is the path to the YAML file, which is applied via `kubectl apply -f`, before creating the service under `YamlLocation`. 

## Deployment File Generation

While the `deploy_info.json` file ships with the `vSwarm_deploy_metadata.tar.gz`, In order to regenerate the `deploy_info.json` run from the root of this repository:
```console
tar -xzvf workloads/container/vSwarm_deploy_metadata.tar.gz -C workloads/container
cd workloads/container/
python3 generate_deploy_info.py
```

## YAML Generation

The `workloads/container/generate_all_yamls.py` is a wrapper script that calls the `generate-yamls.py` script for each vSwarm benchmark in the `vSwarm_deploy_metadata.tar.gz`. The `generate-yamls.py` Python script generates YAML files for vSwarm benchmarks with different workload parameters to create a variety of durations and memory consumption. 

While the YAMLs are pre-generated inside the `vSwarm_deploy_metadata.tar.gz` tarball, to regenerate the YAMLs, run this from the root of this repository:
```console
tar -xzvf workloads/container/vSwarm_deploy_metadata.tar.gz -C workloads/container
cd workloads/container
python3 generate_all_yamls.py
```