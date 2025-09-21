package cluster

import (
	"github.com/vhive-serverless/loader/scripts/setup/configs"
	loaderUtils "github.com/vhive-serverless/loader/scripts/setup/utils"
	"github.com/vhive-serverless/vHive/scripts/utils"
)

func CreateMultiNodeSetup(configDir string) {
	// Load Configurations
	_, extNodeSetup, err := configs.GetNodeSetup(configDir)
	if err != nil {
		utils.FatalPrintf("Failed to get node setup config: %v\n", err)
	}

	setupCfg, err := configs.GetSetupJSON(configDir)
	if err != nil {
		utils.FatalPrintf("Failed to get setup config: %v\n", err)
	}

	minioConfig, err := configs.GetMinioConfig(configDir)
	if err != nil {
		utils.FatalPrintf("Failed to get MinIO config: %v\n", err)
	}

	promConfig, err := configs.GetPromConfig(configDir)
	if err != nil {
		utils.FatalPrintf("Failed to get Prometheus config: %v\n", err)
	}

	masterNode := extNodeSetup.NodeSetup.MasterNode[0]
	workerNodes := extNodeSetup.NodeSetup.WorkerNode
	minioOperatorNodes := extNodeSetup.NodeSetup.MinioOperatorNode
	minioTenantNodes := extNodeSetup.NodeSetup.MinioTenantNode
	allNodes := append([]string{masterNode}, workerNodes...)

	// Distribute Loader SSH Key
	utils.InfoPrintf("Distributing loader SSH key...\n")
	if err := distributeLoaderSSHKey(masterNode, allNodes); err != nil {
		utils.FatalPrintf("Failed to distribute loader SSH key: %v\n", err)
	}
	utils.InfoPrintf("Loader SSH key distributed.\n")

	// Determine Operation Mode
	var operationMode string
	switch setupCfg.ClusterMode {
	case "container":
		operationMode = "stock-only"
	case "firecracker", "firecracker_snapshots":
		operationMode = "firecracker"
	default:
		utils.FatalPrintf("Unsupported cluster mode: %s\n", setupCfg.ClusterMode)
	}

	// Common Initialization on all nodes
	utils.InfoPrintf("Starting common initialization on all nodes...\n")
	if err := commonInit(allNodes, setupCfg, operationMode); err != nil {
		utils.FatalPrintf("Failed during common initialization: %v\n", err)
	}
	utils.InfoPrintf("Common initialization completed.\n")

	// Setup Master Node
	utils.InfoPrintf("Setting up master node: %s\n", masterNode)
	joinToken, err := setupMaster(masterNode, operationMode)
	if err != nil {
		utils.FatalPrintf("Failed to setup master node: %v\n", err)
	}
	utils.InfoPrintf("Master node setup completed.\n")

	// Setup Worker Nodes
	utils.InfoPrintf("Setting up worker nodes...\n")
	if err := setupWorkers(workerNodes, joinToken, setupCfg, operationMode); err != nil {
		utils.FatalPrintf("Failed to setup worker nodes: %v\n", err)
	}
	utils.InfoPrintf("Worker nodes setup completed.\n")

	// Extend CIDR if necessary
	if setupCfg.PodsPerNode > 240 {
		utils.InfoPrintf("Extending CIDR range...\n")
		if err := extendCIDR(masterNode, workerNodes, joinToken); err != nil {
			utils.FatalPrintf("Failed to extend CIDR range: %v\n", err)
		}
		utils.InfoPrintf("CIDR range extended.\n")
	}

	// Finalize Cluster Setup
	utils.InfoPrintf("Finalizing cluster setup...\n")
	if err := finalizeClusterSetup(masterNode, allNodes); err != nil {
		utils.FatalPrintf("Failed to finalize cluster setup: %v\n", err)
	}
	utils.InfoPrintf("Cluster setup finalized.\n")

	// Label Nodes
	utils.InfoPrintf("Labeling nodes...\n")
	if err := loaderUtils.LabelNodes(masterNode, configDir); err != nil {
		utils.FatalPrintf("Failed to label nodes: %v\n", err)
	}
	utils.InfoPrintf("Node labeling completed.\n")

	if setupCfg.DeployMinio {
		utils.InfoPrintf("Setting up MinIO...\n")
		if err := setupMinio(masterNode, minioOperatorNodes, minioTenantNodes, minioConfig); err != nil {
			utils.FatalPrintf("Failed to setup MinIO: %v\n", err)
		}
		utils.InfoPrintf("MinIO setup completed.\n")
	}

	// Deploy Prometheus if enabled
	if setupCfg.DeployPrometheus {
		utils.InfoPrintf("Setting up Prometheus components...\n")
		if err := setupPrometheus(masterNode, allNodes, promConfig); err != nil {
			utils.FatalPrintf("Failed to setup Prometheus components: %v\n", err)
		}
		utils.InfoPrintf("Prometheus components setup completed.\n")
	}

	// Post-Setup Configuration
	utils.InfoPrintf("Applying post-setup configurations...\n")
	if err := applyPostSetupConfigurations(masterNode); err != nil {
		utils.FatalPrintf("Failed to apply post-setup configurations: %v\n", err)
	}
	utils.InfoPrintf("Post-setup configurations applied successfully.\n")

	utils.InfoPrintf("Multi-node cluster setup finished successfully!\n")

}
