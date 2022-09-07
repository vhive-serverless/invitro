#!/bin/bash

pushd $HOME/vhive > /dev/null


# 1. Update IP address range 

pushd configs/metallb/ > /dev/null

cat metallb-configmap.yaml | head -n 11 > metallb-configmap-mod.yaml
echo "      - 10.200.3.4-10.200.3.24" >> metallb-configmap-mod.yaml
    mv metallb-configmap-mod.yaml metallb-configmap.yaml

popd > /dev/null


# 1. Update CPU and memory limits. 
#    NOTE: The commands depend on the order of containers in the YAML file.

pushd configs/istio/ > /dev/null

cat net-istio.yaml | \
    yq e '
    (
        select(
        .spec.template.metadata.labels.app == "net-istio-controller"
        or .spec.template.metadata.labels.app == "net-istio-webhook"
    )
    | .spec.template.spec.containers[0].resources.limits.cpu ) = 3' \
    | yq e '
    (
        select(
        .spec.template.metadata.labels.app == "net-istio-controller"
        or .spec.template.metadata.labels.app == "net-istio-webhook"
    )
    | .spec.template.spec.containers[0].resources.limits.memory ) = "10Gi"' \
    > net-istio-mod.yaml && \
    mv net-istio-mod.yaml net-istio.yaml

popd > /dev/null


# 1. Change calico-node's container env variable CALICO_IPV4POOL_CIDR
#    NOTE: This command creates duplicate entries if ran several times 
#          but it should be ok (no easy fix).
# 2. Add arguments to run flammeld
#    NOTE: The commands depend on the order of containers in the YAML file.

pushd configs/calico > /dev/null

cat canal.yaml | \
    yq e '
    (
        select(
        .spec.template.spec.containers[].name == "calico-node"
    )
    | .spec.template.spec.containers[0].env ) |= . +
    [ {"name": "CALICO_IPV4POOL_CIDR", "value": "10.168.0.0/16"} ]' \
    | yq e '
    (
        select(
        .spec.template.spec.containers[].name == "kube-flannel"
    )
    | .spec.template.spec.containers[1].command ) = ["/opt/bin/flanneld"]' \
    | yq e '
    (
        select(
        .spec.template.spec.containers[].name == "kube-flannel"
    )
    | .spec.template.spec.containers[1].args ) |= 
    [ "--ip-masq", "--kube-subnet-mgr", "--iface=$IFACE"]' \
    > canal-mod.yaml && \
    mv canal-mod.yaml canal.yaml

popd > /dev/null


# 1. Enforce placement on the master node
# 2a,b. Update CPU and memory limits. 
#    NOTE: The commands depend on the order of containers in the YAML file.
# 3. Remove the affinity constraint

for platform in stock vhive
do
    pushd configs/knative_yamls/$platform > /dev/null
    
    cat serving-core.yaml | \
        yq e '
        (
            select(
            .spec.template.metadata.labels.app == "activator"
            or .spec.template.metadata.labels.app == "autoscaler"
            or .spec.template.metadata.labels.app == "controller"
            or .spec.template.metadata.labels.app == "domain-mapping"
            or .spec.template.metadata.labels.app == "domainmapping-webhook"
            or .spec.template.metadata.labels.app == "webhook"
        )
        | .spec.template.spec ) += {"nodeSelector": {"kubernetes.io/hostname":"$MASTER_NODE_NAME"}}' \
        | yq e '
        (
            select(
            .spec.template.metadata.labels.app == "activator"
            or .spec.template.metadata.labels.app == "autoscaler"
            or .spec.template.metadata.labels.app == "controller"
            or .spec.template.metadata.labels.app == "domain-mapping"
            or .spec.template.metadata.labels.app == "domainmapping-webhook"
            or .spec.template.metadata.labels.app == "webhook"
        )
        | .spec.template.spec.containers[0].resources.limits.cpu ) = 3' \
        | yq e '
        (
            select(
            .spec.template.metadata.labels.app == "activator"
            or .spec.template.metadata.labels.app == "autoscaler"
            or .spec.template.metadata.labels.app == "controller"
            or .spec.template.metadata.labels.app == "domain-mapping"
            or .spec.template.metadata.labels.app == "domainmapping-webhook"
            or .spec.template.metadata.labels.app == "webhook"
        )
        | .spec.template.spec.containers[0].resources.limits.memory ) = "10Gi"' \
        | yq e '
        (
            select(
            .spec.template.metadata.labels.app == "activator"
            or .spec.template.metadata.labels.app == "autoscaler"
            or .spec.template.metadata.labels.app == "controller"
            or .spec.template.metadata.labels.app == "domain-mapping"
            or .spec.template.metadata.labels.app == "domainmapping-webhook"
            or .spec.template.metadata.labels.app == "webhook"
        )
        | .spec.template.spec ) |= with_entries(select(.key == "affinity" | not))' \
        > serving-core-mod.yaml && \
        mv serving-core-mod.yaml serving-core.yaml
    
    popd > /dev/null
done

popd > /dev/null # leave the vhive dir
