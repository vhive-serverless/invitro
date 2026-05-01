package cluster

import (
	"fmt"

	"github.com/vhive-serverless/loader/scripts/setup/configs"
	loaderUtils "github.com/vhive-serverless/loader/scripts/setup/utils"
	"github.com/vhive-serverless/vHive/scripts/utils"
)

func setupKNBill(masterNode string, allNode []string, knbillConfig *configs.KnbillConfig) error {

	// Patch Queue-Proxy deployment to use custom image
	utils.WaitPrintf("Patching Queue-Proxy deployment to use custom image on master node %s...\n", masterNode)
	_, err := loaderUtils.ServerExec(masterNode, fmt.Sprintf(`kubectl patch image queue-proxy -n knative-serving --type='merge' -p '{"spec":{"image":"%s"}}'`, knbillConfig.QueueProxyImage))
	if !utils.CheckErrorWithMsg(err, "Failed to patch Queue-Proxy deployment on master node %s: %v\n", masterNode, err) {
		return err
	}

	_, err = loaderUtils.ServerExec(masterNode, `kubectl patch deployment activator -n knative-serving -p '{"spec": {"template": {"spec": {"containers": [{"name": "activator", "image": "nehalem90/activator-ecd51ca5034883acbe737fde417a3d86:latest", "imagePullPolicy": "Always"}]}}}}'`)
	if err != nil {
		return err
	}

	// Create Billing namespace
	utils.WaitPrintf("Creating billing namespace on master node %s...\n", masterNode)
	_, err = loaderUtils.ServerExec(masterNode, "kubectl create namespace billing")
	if !utils.CheckErrorWithMsg(err, "Failed to create billing namespace on master node %s: %v\n", masterNode, err) {
		return err
	}

	// Build Billet manifest from template
	utils.WaitPrintf("Building Billet manifest from template %s...\n", masterNode)
	_, err = loaderUtils.ServerExec(masterNode, fmt.Sprintf("cat %s/billet_daemonset_template.yaml | BILLET_IMAGE=\"%s\" envsubst > %s/billet-daemonset.yaml", knbillConfig.KnbillPath, knbillConfig.BilletImage, knbillConfig.KnbillPath))
	if !utils.CheckErrorWithMsg(err, "Failed to deploy Knbill Billet component on master node %s: %v\n", masterNode, err) {
		return err
	}

	// Deploy Billet DaemonSet
	utils.WaitPrintf("Deploying Billet DaemonSet %s...\n", masterNode)
	_, err = loaderUtils.ServerExec(masterNode, fmt.Sprintf("kubectl apply -f %s/billet-daemonset.yaml -n billing", knbillConfig.KnbillPath))
	if !utils.CheckErrorWithMsg(err, "Failed to apply Knbill Billet DaemonSet manifest on master node %s: %v\n", masterNode, err) {
		return err
	}

	return nil
}
