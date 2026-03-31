package cluster

import (
	loaderUtils "github.com/vhive-serverless/loader/scripts/setup/utils"
)

func applyPostSetupConfigurations(masterNode string) error {

	_, err := loaderUtils.ServerExec(masterNode, "curl -sS https://webi.sh/k9s | sh")
	if err != nil {
		return err
	}

	_, err = loaderUtils.ServerExec(masterNode, `kubectl patch clusterrole knative-serving-activator-cluster --type='json' -p '[{"op": "add", "path": "/rules/-", "value": {"apiGroups": [""], "resources": ["nodes"], "verbs": ["get", "list", "watch"]}}]'`)
	if err != nil {
		return err
	}

	_, err = loaderUtils.ServerExec(masterNode, `kubectl patch deployment activator -n knative-serving -p '{"spec": {"template": {"spec": {"containers": [{"name": "activator", "image": "nehalem90/activator-ecd51ca5034883acbe737fde417a3d86:latest", "imagePullPolicy": "Always"}]}}}}'`)
	if err != nil {
		return err
	}

	_, err = loaderUtils.ServerExec(masterNode, `kubectl set env deployment/activator -n knative-serving KEEPALIVE_DURATION=83 UPDATE_INTERVAL=13`)
	if err != nil {
		return err
	}

	_, err = loaderUtils.ServerExec(masterNode, `kubectl rollout restart -n knative-serving deployment/activator`)
	if err != nil {
		return err
	}

	_, err = loaderUtils.ServerExec(masterNode, `kubectl patch cm config-deployment -n knative-serving -p '{"data": {"queue-sidecar-cpu-request": "10m"}}'`)
	if err != nil {
		return err
	}

	_, err = loaderUtils.ServerExec(masterNode, `kubectl set resources deployment istio-ingressgateway -n istio-system -c istio-proxy --limits=cpu=4,memory="4Gi" --requests=cpu=100m,memory=128Mi`)
	if err != nil {
		return err
	}

	_, err = loaderUtils.ServerExec(masterNode, `kubectl set resources deployment cluster-local-gateway -n istio-system -c istio-proxy --limits=cpu=4,memory="4Gi" --requests=cpu=100m,memory=128Mi`)
	if err != nil {
		return err
	}

	_, err = loaderUtils.ServerExec(masterNode, `kubectl set resources deployment coredns -n kube-system -c coredns --requests=cpu=100m,memory=128Mi --limits=memory=512Mi`)
	if err != nil {
		return err
	}

	_, err = loaderUtils.ServerExec(masterNode, `kubectl patch deployment istio-ingressgateway -n istio-system --type merge -p '{"spec":{"template":{"metadata":{"annotations":{"proxy.istio.io/config":"{\"concurrency\": 4}"}}}}}'`)

	_, err = loaderUtils.ServerExec(masterNode, `kubectl patch deployment cluster-local-gateway -n istio-system --type merge -p '{"spec":{"template":{"metadata":{"annotations":{"proxy.istio.io/config":"{\"concurrency\": 4}"}}}}}'`)

	_, err = loaderUtils.ServerExec(masterNode, `kubectl patch hpa istio-ingressgateway -n istio-system --type merge -p '{"spec":{"maxReplicas": 20}}'`)

	_, err = loaderUtils.ServerExec(masterNode, `kubectl patch hpa cluster-local-gateway -n istio-system --type merge -p '{"spec":{"maxReplicas": 20}}'`)

	return nil
}
