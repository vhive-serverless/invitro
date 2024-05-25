### 1. Modify log level

First, use the following command to edit the `config-logging` ConfigMap in the `knative-serving` namespace:

```bash
kubectl edit configmap config-logging -n knative-serving
```

Then, add the following two lines to set the log level of the autoscaler and activator components to debug:

```bash
loglevel.autoscaler: debug
loglevel.activator: debug
```

Need to increase log size on autoscaler and activator nodes

```bash
sudo echo "containerLogMaxSize: 512Mi" > >(sudo tee -a /var/lib/kubelet/config.yaml >/dev/null)
sudo systemctl restart kubelet
```

### 2. Modify parameters to solve bottlenecks

```bash
sudo vim /etc/kubernetes/manifests/kube-controller-manager.yaml
--kube-api-qps=1000
--kube-api-burst=2000

sudo vim /etc/kubernetes/manifests/etcd.yaml
--quota-backend-bytes=16000000000
```

### 3. Download the dataset

```bash
git lfs pull
mkdir ~/traces
tar -xzf data/traces/reference/sampled_150.tar.gz -C ~/traces
```

### 4. Execute run.sh.

Please note that I have set many Istio parameters in run.sh to address bottlenecks. Please check run.sh.

```bash
scripts/setup/run.sh
```