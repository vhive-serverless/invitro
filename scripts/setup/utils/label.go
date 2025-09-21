package utils

import (
	"fmt"
	"strings"

	"github.com/vhive-serverless/loader/scripts/setup/configs"
	"github.com/vhive-serverless/vHive/scripts/utils"
)

// LabelNodes applies labels to Kubernetes nodes based on the configuration file using kubectl commands.
// masterNode is the IP address of the master node where kubectl commands will be executed.
// configDir specifies the directory where `node_setup.json` is located.
func LabelNodes(masterNode, configDir string) error {
	_, extNodeSetup, err := configs.GetNodeSetup(configDir)
	if !utils.CheckErrorWithMsg(err, "Failed to get node setup config from %s: %v", configDir, err) {
		return err
	}

	for label, nodeIPs := range extNodeSetup.NodeLabel {
		for _, nodeIP := range nodeIPs {
			// Get the hostname of the node from its IP
			nodeName, err := ServerExec(nodeIP, "hostname")
			if !utils.CheckErrorWithMsg(err, "Failed to get hostname for node IP %s: %v", nodeIP, err) {
				// Continue labeling other nodes even if one fails
				utils.FatalPrintf("Error getting hostname for node IP %s: %v\n", nodeIP, err)
				continue
			}
			nodeName = strings.TrimSpace(nodeName)

			// Apply the label using kubectl on the master node
			labelCmd := fmt.Sprintf("kubectl label nodes %s %s --overwrite", nodeName, label)
			_, err = ServerExec(masterNode, labelCmd)
			if !utils.CheckErrorWithMsg(err, "Failed to label node %s with %s on master node %s: %v", nodeName, label, masterNode, err) {
				// Continue labeling other nodes even if one fails
				utils.FatalPrintf("Error labeling node %s with %s: %v\n", nodeName, label, err)
			} else {
				utils.InfoPrintf("Successfully labeled node %s with %s\n", nodeName, label)
			}
		}
	}

	return nil
}
