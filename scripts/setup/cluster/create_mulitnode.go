package cluster

import (
	"github.com/vhive-serverless/loader/scripts/setup/configs"
	loaderUtils "github.com/vhive-serverless/loader/scripts/setup/utils"
	"github.com/vhive-serverless/vHive/scripts/utils"
)

func CreateMultiNodeSetup(configDir string, configName string) {
	// Load Configurations
	cfg, err := configs.CommonConfigSetup(configDir, configName)
	if err != nil {
		utils.FatalPrintf("Failed to load configurations: %v\n", err)
	}

	// Distribute Loader SSH Key
	utils.InfoPrintf("Distributing loader SSH key...\n")
	if err := distributeLoaderSSHKey(cfg.MasterNode, cfg.AllNodes); err != nil {
		utils.FatalPrintf("Failed to distribute loader SSH key: %v\n", err)
	}
	utils.InfoPrintf("Loader SSH key distributed.\n")

	// Determine Operation Mode
	var operationMode string
	switch cfg.SetupCfg.ClusterMode {
	case "container":
		operationMode = "stock-only"
	case "firecracker", "firecracker_snapshots":
		operationMode = "firecracker"
	default:
		utils.FatalPrintf("Unsupported cluster mode: %s\n", cfg.SetupCfg.ClusterMode)
	}

	// Common Initialization on all nodes
	utils.InfoPrintf("Starting common initialization on all nodes...\n")
	if err := commonInit(cfg.AllNodes, cfg.SetupCfg, operationMode); err != nil {
		utils.FatalPrintf("Failed during common initialization: %v\n", err)
	}
	utils.InfoPrintf("Common initialization completed.\n")

	// Setup Master Node
	utils.InfoPrintf("Setting up master node: %s\n", cfg.MasterNode)
	joinToken, err := setupMaster(cfg.MasterNode, operationMode)
	if err != nil {
		utils.FatalPrintf("Failed to setup master node: %v\n", err)
	}
	utils.InfoPrintf("Master node setup completed.\n")

	// Setup Worker Nodes
	utils.InfoPrintf("Setting up worker nodes...\n")
	if err := setupWorkers(cfg.WorkerNodes, joinToken, cfg.SetupCfg, operationMode); err != nil {
		utils.FatalPrintf("Failed to setup worker nodes: %v\n", err)
	}
	utils.InfoPrintf("Worker nodes setup completed.\n")

	// Extend CIDR if necessary
	if cfg.SetupCfg.PodsPerNode > 240 {
		utils.InfoPrintf("Extending CIDR range...\n")
		if err := extendCIDR(cfg.MasterNode, cfg.WorkerNodes, joinToken); err != nil {
			utils.FatalPrintf("Failed to extend CIDR range: %v\n", err)
		}
		utils.InfoPrintf("CIDR range extended.\n")
	}

	// Finalize Cluster Setup
	utils.InfoPrintf("Finalizing cluster setup...\n")
	if err := finalizeClusterSetup(cfg.MasterNode, cfg.AllNodes); err != nil {
		utils.FatalPrintf("Failed to finalize cluster setup: %v\n", err)
	}
	utils.InfoPrintf("Cluster setup finalized.\n")

	// Label Nodes
	utils.InfoPrintf("Labeling nodes...\n")
	if err := loaderUtils.LabelNodes(cfg.MasterNode, configDir, configName); err != nil {
		utils.FatalPrintf("Failed to label nodes: %v\n", err)
	}
	utils.InfoPrintf("Node labeling completed.\n")

	// Deploy Prometheus if enabled
	if cfg.SetupCfg.DeployPrometheus {
		utils.InfoPrintf("Setting up Prometheus components...\n")
		if err := setupPrometheus(cfg.MasterNode, cfg.AllNodes, cfg.PromConfig); err != nil {
			utils.FatalPrintf("Failed to setup Prometheus components: %v\n", err)
		}
		utils.InfoPrintf("Prometheus components setup completed.\n")
	}

	if cfg.SetupCfg.DeployMinio {
		utils.InfoPrintf("Setting up MinIO...\n")
		if err := setupMinio(cfg.MasterNode, cfg.MinioOperatorNodes, cfg.MinioTenantNodes, cfg.MinioConfig); err != nil {
			utils.FatalPrintf("Failed to setup MinIO: %v\n", err)
		}
		utils.InfoPrintf("MinIO setup completed.\n")
	}

	// Post-Setup Configuration
	utils.InfoPrintf("Applying post-setup configurations...\n")
	if err := applyPostSetupConfigurations(cfg.MasterNode); err != nil {
		utils.FatalPrintf("Failed to apply post-setup configurations: %v\n", err)
	}
	utils.InfoPrintf("Post-setup configurations applied successfully.\n")

	utils.InfoPrintf("Multi-node cluster setup finished successfully!\n")

}
