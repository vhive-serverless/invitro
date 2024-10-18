# Setup steps for running the loader with OpenWhisk

## Creating a multi-node cluster

First, configure `scripts/setup/setup.cfg`. You can specify there which vHive branch to use, loader branch, operation mode
(sandbox type), maximum number of pods per node, and the Github token. All these configurations are mandatory.
We currently support the following modes: containerd (`container`), Firecracker (`firecracker`), and Firecracker with
snapshots (`firecracker_snapshots`).
The token needs `repo` and `admin:public_key` permissions and will be used for adding SSH key to the user's account for
purpose of cloning Github private repositories.
Loader will be cloned on every node specified as argument of the cluster create script. The same holds for Kubernetes
API server certificate.  

* To create a multi-node cluster, specify the node addresses as the arguments and run the following command: 

```bash
$ bash  ./scripts/setup/create_multinode.sh <master_node@IP> <worker_node@IP> ...
``` 

The loader should be running on a separate node that is a part of the Kubernetes cluster. Do not collocate the master and
worker node components with the loader for performance reasons.

After the `create_multinode.sh` script completes, run the following two commands to delete the namespaces related to Knative: 

```bash
$ kubectl  delete  namespace  knative-serving
$ kubectl  delete  namespace  istio-system
```

After the namespaces are deleted, clone the OpenWhisk Kubernetes deployment repository:

```bash
$ git  clone  https://github.com/apache/openwhisk-deploy-kube.git
```

Then, you need to get Helm as well: 

```bash
$ curl  -fsSL  -o  get_helm.sh  https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3
$ chmod  700  get_helm.sh
$ ./get_helm.sh
``` 

Now you need to label all of the nodes in the cluster that should execute functions (worker nodes):

```bash
$ kubectl  label  node <node_name> openwhisk-role=invoker
```

### Setting up necessary files

Next, you need to create a `.yaml` file to describe your cluster. An example of such a file can be found at `openwhisk_setup/mycluster.yaml`. You need to replace `<master_node_public_IP>` with the public IP address of the master node in the cluster.

The next step is setting up the OpenWhisk environment. You need to pay attention to the following three files: 

*  `/openwhisk-deploy-kube/helm/openwhisk/values.yaml`

An example of this file you can use can be found at `openwhisk_setup/values.yaml`

If you want to configure it yourself, follow the instructions below.

Set `whisk.limits.actionsInvokesPerminute` to a value large enough to satisfy your needs (default: 60). 

Set `whisk.limits.actionsInvokesConcurrent` to a value large enough to satisfy your needs (default: 30). 

Set `whisk.containerPool.userMemory` to a value large enough to satisfy your needs (default: 2048m).

Set `invoker.containerFactory.kubernetes.replicaCount` to the number of worker nodes in your cluster (default: 1).

Set `invoker.options` to `"-Dwhisk.spi.LogStoreProvider=org.apache.openwhisk.core.containerpool.logging.LogDriverLogStoreProvider"`. 

In this file you can control the log verbosity by setting the adequate `loglevel` fields to a desired value (default: `"INFO"`).

*  `openwhisk-deploy-kube/helm/openwhisk/runtimes.json` 

An example of this file you can use can be found at `openwhisk_setup/runtimes.json`

If you want to configure it yourself, follow the instructions below.

In this file you configure the policies for keeping pre-warmed container pools. Policies are defined for each supported runtime.

If you want to minimize the usage of the pool (induce cold starts) use the `runtimes.go.stemCells` configuration from the provided sample file.

Details on controlling the pre-warmed container pool size in OpenWhisk can be found [on their GitHub repository](https://github.com/apache/openwhisk/blob/master/docs/actions.md#How%20prewarm%20containers%20are%20provisioned%20without%20a%20reactive%20configuration).

*  `openwhisk-deploy-kube/helm/openwhisk/templates/invoker-pod.yaml`

An example of this file you can use can be found at `openwhisk_setup/invoker-pod.yaml`

If you want to configure it yourself, follow the instructions below.

If you want to change the lifetime of the container after executing a function, you need to add the following two lines after line 132 in the file:

```yaml
- name: "CONFIG_whisk_containerProxy_timeouts_idleContainer"
  value: "<desired_time>"
```

Set the `<desired_time>` to your preferred value.  

### Starting the custom OpenWhisk configuration 

Execute the following two commands to configure helm:  

```bash
$ helm  repo  add  openwhisk  https://openwhisk.apache.org/charts
$ helm  repo  update
```  

Then install the OpenWhisk configuration:

```bash
$ helm  install <dev_name> /openwhisk-deploy-kube/helm/openwhisk  -n  openwhisk  --create-namespace  -f <path_to_mycluster.yaml>
```

`dev_name` can be any name you wish, and `<path_to_mycluster.yaml>` must lead to your `mycluster.yaml` file.
  
You can use the following command to monitor the pod creation process:

```bash
$ kubectl  get  pods  -n  openwhisk  --watch
```

## Installing the wsk CLI

Firstly, download the adequate release, unpack it and move it to the correct location:

```bash
$ wget  https://github.com/apache/openwhisk-cli/releases/download/1.2.0/OpenWhisk_CLI-1.2.0-linux-amd64.tgz
$ tar  zxvf  OpenWhisk_CLI-1.2.0-linux-amd64.tgz
$ sudo  cp  ./wsk  /bin/wsk
```

Set the necessary properties:

```bash
$ wsk  property  set  --apihost <master_node_public_IP>
$ wsk  property  set  --auth  23bc46b1-71f6-4ed5-8c54-816aa4f8c502:123zO3xZCLrMN6v2BKK1dXYFpXlPkccOFqm12CdAsMgRU4VrNZ9lyGVCGuMDGIwP
```  

## Single execution  

First go to `cmd/config_knative_trace.json` and set the `Platform` parameter to `OpenWhisk`.

Then, to run load generator use the following command:

```bash
$ go  run  cmd/loader.go  --config  cmd/config_knative_trace.json
```

Additionally, one can specify log verbosity argument as `--verbosity [info, debug, trace]`. The default value is `info`.

