#!/bin/bash

KNATIVE_VERSION="1.9.0"
master_node_name=$(hostname)
MASTER_NODE_IP=$(ip route | awk '{print $(NF)}' | awk '/^10\..*/')
IFACE=$(netstat -ie | grep -B1 $MASTER_NODE_IP | head -n1 | awk '{print $1}' | cut -d ':' -f 1)

# we set these limits high enough but to fit in the budget of a typical master node server
cpu_limit_net_istio=2
memory_limit_net_istio="30Gi"
cpu_limit_serving_core=28
memory_limit_serving_core="30Gi"
cpu_requests_serving_core=3

pushd $HOME/vhive/configs/knative_yamls >/dev/null

rm serving-core-firecracker.yaml
rm serving-default-domain.yaml
rm serving-hpa.yaml
rm serving-post-install-jobs.yaml
rm serving-storage-version-migration.yaml
rm ../metallb/metallb-ipaddresspool.yaml
rm ../setup/kube.json

wget -q https://github.com/knative-extensions/net-istio/releases/download/knative-v${KNATIVE_VERSION}/net-istio.yaml
wget -q https://github.com/knative/serving/releases/download/knative-v${KNATIVE_VERSION}/serving-core.yaml && mv serving-core.yaml serving-core-firecracker.yaml
wget -q https://github.com/knative/serving/releases/download/knative-v${KNATIVE_VERSION}/serving-core.yaml
wget -q https://github.com/knative/serving/releases/download/knative-v${KNATIVE_VERSION}/serving-crds.yaml
wget -q https://github.com/knative/serving/releases/download/knative-v${KNATIVE_VERSION}/serving-default-domain.yaml
wget -q https://github.com/knative/serving/releases/download/knative-v${KNATIVE_VERSION}/serving-hpa.yaml
wget -q https://github.com/knative/serving/releases/download/knative-v${KNATIVE_VERSION}/serving-post-install-jobs.yaml
wget -q https://github.com/knative/serving/releases/download/knative-v${KNATIVE_VERSION}/serving-storage-version-migration.yaml

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
        ) | .spec.template.spec 
    ) += {"nodeSelector": {"kubernetes.io/hostname":"'"${master_node_name}"'"}}' |
    yq '
    (
        del
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
        )
    )' |
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
        ) | .spec.template.spec.containers[0].resources.requests.cpu
    ) = '"${cpu_requests_serving_core}"' ' |
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
sed -e '$d' > serving-core-yq.yaml

# serving-core-firecracker.yaml

cat serving-core-firecracker.yaml |
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
        ) | .spec.template.spec 
    ) += {"nodeSelector": {"kubernetes.io/hostname":"'"${master_node_name}"'"}}' |
    yq '
    (
        del
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
        )
    )' |
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
            .metadata.labels."app.kubernetes.io/component" == "queue-proxy"
        ) | .spec.image 
    ) = "ghcr.io/vhive-serverless/queue-39be6f1d08a095bd076a71d288d295b6@sha256:41259c52c99af616fae4e7a44e40c0e90eb8f5593378a4f3de5dbf35ab1df49c"' |
    yq '
    (
        select
        (
            .metadata.name == "config-deployment"
        ) | .data.queueSidecarImage 
    ) = "ghcr.io/vhive-serverless/queue-39be6f1d08a095bd076a71d288d295b6@sha256:41259c52c99af616fae4e7a44e40c0e90eb8f5593378a4f3de5dbf35ab1df49c"' |
    yq ' 
    (
        select
        (
            .spec.template.metadata.labels.app == "activator"
        ) | .spec.template.spec.containers[0].image
    ) = "ghcr.io/vhive-serverless/activator-ecd51ca5034883acbe737fde417a3d86@sha256:abf65c8cb9598c6e7c5afb6186618b40afdd75d15b762f44910a0d8a718d953c"' |
    yq ' 
    (
        select
        (
            .spec.template.metadata.labels.app == "autoscaler"
        ) | .spec.template.spec.containers[0].image
    ) = "ghcr.io/vhive-serverless/autoscaler-12c0fa24db31956a7cfa673210e4fa13@sha256:76e66b57c8233f9852a719f17b7727bb7d70988cf0cc3a7c86b57e7fdaf1b328"' |
    yq ' 
    (
        select
        (
            .spec.template.metadata.labels.app == "controller"
        ) | .spec.template.spec.containers[0].image
    ) = "ghcr.io/vhive-serverless/controller-f6fdb41c6acbc726e29a3104ff2ef720@sha256:5496a4fdb200544d88a099aa049bc8cb107b20d0b7da05da241e502ae3d89620"' |
    yq ' 
    (
        select
        (
            .spec.template.metadata.labels.app == "domain-mapping"
        ) | .spec.template.spec.containers[0].image
    ) = "ghcr.io/vhive-serverless/domain-mapping-82f8626be89c35bcd6c666fd2fc8ccb7@sha256:6d34ad9e43c2052408e2143017c9a6dadd17beedb7723f24ae8a93a484165127"' |
    yq ' 
    (
        select
        (
            .spec.template.metadata.labels.app == "domainmapping-webhook"
        ) | .spec.template.spec.containers[0].image
    ) = "ghcr.io/vhive-serverless/domain-mapping-webhook-a127ae3e896f4e4bc1179581011b7824@sha256:cb682d3b328bcd2ecfc75479bba436af6718d3bf5f2bc28080e6b8ab7cbe7008"' |
    yq ' 
    (
        select
        (
            .spec.template.metadata.labels.app == "webhook"
        ) | .spec.template.spec.containers[0].image
    ) = "ghcr.io/vhive-serverless/webhook-261c6506fca17bc41be50b3461f98f1c@sha256:2c559264619d779884b5d3121d7214efc22df54b716fa843c94af29e38eb6238"' |
sed -e 's/"'"${KNATIVE_VERSION}"'"/devel/g' | sed -e '$d' > serving-core-firecracker-yq.yaml


# serving-crds.yaml

cat serving-crds.yaml | sed -e 's/"'"${KNATIVE_VERSION}"'"/devel/g' > serving-crds-yq.yaml

# serving-default-domain.yaml

cat serving-default-domain.yaml | 
    yq ' 
    (
        select
        (
            .spec.template.metadata.labels.app == "default-domain"
        ) | .spec.template.spec.containers[0].image
    ) = "ghcr.io/vhive-serverless/default-domain-dd6f55160290a90018ba318b624be12e@sha256:0206e792ee18591903e8b8aaaba92d04548c9b63d9742a98195037697bdbc42e"' |
sed -e 's/"'"${KNATIVE_VERSION}"'"/devel/g' | sed -e '$d' > serving-default-domain-yq.yaml

# serving-hpa.yaml

cat serving-hpa.yaml |  
    yq '
    (
        select
        (
            .spec.template.metadata.labels.app == "autoscaler-hpa"
        ) | .spec.template.spec.containers[0].image
    ) = "ghcr.io/vhive-serverless/autoscaler-hpa-85c0b68178743d74ff7f663a72802ceb@sha256:1797f6c16f0c65b0255651f82e85399ba210cc353044177ad2af159fc20e2122"' |
sed -e 's/"'"${KNATIVE_VERSION}"'"/devel/g' | sed -e '$d' > serving-hpa-yq.yaml

# serving-post-install-jobs.yaml

cat serving-post-install-jobs.yaml |
    yq '
    (
        select
        (
            .spec.template.metadata.labels.app == "storage-version-migration-serving"
        ) | .spec.template.spec.containers[0].image
    ) = "ghcr.io/vhive-serverless/migrate-242d0a35bf580c5b411a545d79618fbf@sha256:2d2159119a4c3bd8470a14a970835aa1f56a64250a9ff8bc59ea534175f8548b"' |
sed -e 's/"'"${KNATIVE_VERSION}"'"/devel/g' | sed -e '$d' > serving-post-install-jobs-yq.yaml

# serving-storage-version-migration.yaml

cat serving-storage-version-migration.yaml |
    yq '
    (
        select
        (
            .spec.template.metadata.labels.app == "storage-version-migration-serving"
        ) | .spec.template.spec.containers[0].image
    ) = "ghcr.io/vhive-serverless/migrate-242d0a35bf580c5b411a545d79618fbf@sha256:2d2159119a4c3bd8470a14a970835aa1f56a64250a9ff8bc59ea534175f8548b"' |
sed -e 's/"'"${KNATIVE_VERSION}"'"/devel/g' | sed -e '$d' > serving-storage-version-migration-yq.yaml


# canal.yaml

cat ../calico/canal.yaml | \
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
sed -e 's/{}//g' > canal-yq.yaml && rm ../calico/canal.yaml && mv canal-yq.yaml ../calico/canal.yaml

mv net-istio-yq.yaml net-istio.yaml
mv serving-core-yq.yaml serving-core.yaml
mv serving-core-firecracker-yq.yaml serving-core-firecracker.yaml
mv serving-crds-yq.yaml serving-crds.yaml
mv serving-default-domain-yq.yaml serving-default-domain.yaml
mv serving-hpa-yq.yaml serving-hpa.yaml
mv serving-post-install-jobs-yq.yaml serving-post-install-jobs.yaml
mv serving-storage-version-migration-yq.yaml serving-storage-version-migration.yaml
mv ~/loader/config/metallb-ipaddresspool.yaml ../metallb/metallb-ipaddresspool.yaml
mv ~/loader/config/kube.json ../setup/kube.json

popd >/dev/null # leave the vhive dir
