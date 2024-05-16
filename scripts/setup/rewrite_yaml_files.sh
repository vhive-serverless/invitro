#!/bin/bash

KNATIVE_VERSION=`jq -r '.KnativeVersion' < ~/vhive/configs/setup/knative.json`
CALICO_VERSION=`jq -r '.CalicoVersion' < ~/loader/config/kube.json`

MASTER_NODE_IP=$(ip route | awk '{print $(NF)}' | awk '/^10\..*/')
IFACE=$(netstat -ie | grep -B1 $MASTER_NODE_IP | head -n1 | awk '{print $1}' | cut -d ':' -f 1)

# we set these limits high enough but to fit in the budget of a typical master node server
cpu_limit_net_istio=2
memory_limit_net_istio="10Gi"
cpu_limit_serving_core=3
memory_limit_serving_core="10Gi"

pushd $HOME/vhive/configs >/dev/null
mkdir knative_yamls -p
cd knative_yamls

wget -q https://github.com/knative-extensions/net-istio/releases/download/knative-v${KNATIVE_VERSION}/net-istio.yaml
wget -q https://github.com/knative/serving/releases/download/knative-v${KNATIVE_VERSION}/serving-core.yaml
wget -q https://raw.githubusercontent.com/projectcalico/calico/v${CALICO_VERSION}/manifests/calico.yaml -P ../calico

# net-istio.yaml

cat net-istio.yaml |
    yq '
    (
        select
        (
               .spec.selector.matchLabels.app == "net-istio-controller"
            or .spec.selector.matchLabels.role == "net-istio-webhook"
        )
        | .spec.template.spec.containers[0].resources.limits.cpu
    ) = '"${cpu_limit_net_istio}"' ' |
    yq '
    (
        select
        (
               .spec.selector.matchLabels.app == "net-istio-controller"
            or .spec.selector.matchLabels.role == "net-istio-webhook"
        )
        | .spec.template.spec.containers[0].resources.limits.memory
    ) = "'"${memory_limit_net_istio}"'" ' |
sed -e '$d' > net-istio-yq.yaml

# serving-core.yaml

cat serving-core.yaml |
    yq '
    (
        select
        (
               .spec.template.metadata.labels.app == "activator"
            or .spec.template.metadata.labels.app == "autoscaler"
            or .spec.template.metadata.labels.app == "controller"
            or .spec.template.metadata.labels.app == "domain-mapping"
            or .spec.template.metadata.labels.app == "domainmapping-webhook"
            or .spec.template.metadata.labels.app == "webhook"
        ) | .spec.template.spec.affinity
    ) = {"nodeAffinity": {"requiredDuringSchedulingIgnoredDuringExecution": {"nodeSelectorTerms": [{"matchExpressions": [{"key": "loader-nodetype", "operator": "In", "values": ["master", "singlenode"]}]}]}}}' |
    yq '
    (
        select
        (
                .spec.template.metadata.labels.app == "activator"
            or .spec.template.metadata.labels.app == "autoscaler"
            or .spec.template.metadata.labels.app == "controller"
            or .spec.template.metadata.labels.app == "domain-mapping"
            or .spec.template.metadata.labels.app == "domainmapping-webhook"
            or .spec.template.metadata.labels.app == "webhook"
        ) | .spec.template.spec.containers[0].resources.limits.cpu
    ) = '"${cpu_limit_serving_core}"' ' |
    yq '
    (
        select
        (
                .spec.template.metadata.labels.app == "activator"
            or .spec.template.metadata.labels.app == "autoscaler"
            or .spec.template.metadata.labels.app == "controller"
            or .spec.template.metadata.labels.app == "domain-mapping"
            or .spec.template.metadata.labels.app == "domainmapping-webhook"
            or .spec.template.metadata.labels.app == "webhook"
        ) | .spec.template.spec.containers[0].resources.limits.memory 
    ) = "'"${memory_limit_serving_core}"'" ' |
    yq '
    (
        select
        (
            .spec.template.metadata.labels.app == "autoscaler"
        ) | .spec.template.spec.containers[0].env
    ) += [{"name": "KUBE_API_BURST", "value": "20"}, {"name": "KUBE_API_QPS", "value": "10"}]' |
    yq '
    (
        select
        (
            .spec.template.metadata.labels.app == "autoscaler"
        ) | .spec.template.spec.containers[0].image
    ) = "lkondras/autoscaler-12c0fa24db31956a7cfa673210e4fa13:base"' |
    yq '
    (
        select
        (
            .spec.template.metadata.labels.app == "activator"
        ) | .spec.template.spec.containers[0].image
    ) = "lkondras/activator-ecd51ca5034883acbe737fde417a3d86:rr-policy"' |
sed -e '$d' > serving-core-yq.yaml

# calico.yaml

cat ../calico/calico.yaml | \
    yq '
    (
        select
        (
            .spec.template.spec.containers[].name == "calico-node"
        ) | .spec.template.spec.containers[0].env 
    ) |= . + [ {"name": "CALICO_IPV4POOL_CIDR", "value": "10.168.0.0/16"} ]' | 
    yq '
    (
        select
        (           
                .metadata.name == "calico-config"
            and .metadata.namespace == "kube-system"
        ) | .data.canal_iface
    ) = "'"${IFACE}"'" ' |
sed -e 's/{}//g' > calico-yq.yaml

mv net-istio-yq.yaml net-istio.yaml
mv serving-core-yq.yaml serving-core.yaml
mv calico-yq.yaml ../calico/calico.yaml
cp ~/loader/config/metallb-ipaddresspool.yaml ../metallb/metallb-ipaddresspool.yaml
cp ~/loader/config/kube.json ../setup/kube.json
cp ~/loader/config/system.json ../setup/system.json

popd >/dev/null # leave the vhive dir
