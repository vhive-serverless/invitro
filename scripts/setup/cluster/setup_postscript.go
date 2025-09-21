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

	_, err = loaderUtils.ServerExec(masterNode, `kubectl patch deployment activator -n knative-serving -p '{"spec": {"template": {"spec": {"containers": [{"name": "activator", "image": "nivekiba/activator-ecd51ca5034883acbe737fde417a3d86:final", "imagePullPolicy": "Always"}]}}}}'`)
	if err != nil {
		return err
	}

	_, err = loaderUtils.ServerExec(masterNode, `kubectl set env deployment/activator -n knative-serving KEEPALIVE_DURATION=60 UPDATE_INTERVAL=5`)
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

	return nil
}
