# Deploy Info JSON

## The `deploy_info.json` file in the `yamls.tar.gz`  is used to identify the relative file paths for the Knative YAML manifests for deploying vSwarm functions. It is generated using `generate_deploy_info.py` Python script, also present under `yamls.tar.gz`, which outputs a JSON that embeds the yaml-location and pre-deployment commands for every vSwarm function. zit also contains the path of YAML files needed as part of the pre-deployment commands to run certain vSwarm benchmarks, for example the `online-shop-database` which requires to be deployed before running `cartservice` benchmark.

## While the `deploy_info.json` file ships with the `yamls.tar.gz`, In order to regenerate the `deploy_info.json` run:
```console
tar -xzvf workloads/container/vSwarm_deploy_metadata.tar.gz -C workloads/container
cd workloads/container/yamls/
python3 generate_deploy_info.py
```

The `deploy_info.json` has the following schema:
```console
{
    vswarm-function-name:
        {
            YamlLocation: /path/to/yaml
            PredeploymentPath: [/path/to/predeployment/yaml]
        }
}
```