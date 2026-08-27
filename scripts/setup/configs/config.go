package configs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/vhive-serverless/vHive/scripts/utils"
)

type InvitroSetupConfig struct {
	MasterNode         string
	LoaderNode         string
	WorkerNodes        []string
	MinioOperatorNodes []string
	MinioTenantNodes   []string
	AllNodes           []string

	SetupCfg    *SetupConfig
	MinioConfig *MinioConfig
	PromConfig  *PrometheusConfig
}

type NodeSetup struct {
	NodeSetup struct {
		MasterNode        []string `json:"MASTER_NODE"`
		LoaderNode        []string `json:"LOADER_NODE"`
		WorkerNode        []string `json:"WORKER_NODE"`
		MinioOperatorNode []string `json:"MINIO_OPERATOR_NODE"`
		MinioTenantNode   []string `json:"MINIO_TENANT_NODE"`
	} `json:"NODE_SETUP"`
	NodeLabel map[string][]string `json:"NODE_LABEL"`
	NodeURL   []string            `json:"NODE_URL"`
}

type SetupConfig struct {
	HiveRepo          string `json:"VHIVE_REPO"`
	HiveBranch        string `json:"VHIVE_BRANCH"`
	LoaderRepo        string `json:"LOADER_REPO"`
	LoaderBranch      string `json:"LOADER_BRANCH"`
	KhalaRepo         string `json:"KHALA_REPO"`
	KhalaBranch       string `json:"KHALA_BRANCH"`
	FirecrackerRepo   string `json:"FIRECRACKER_REPO"`
	FirecrackerBranch string `json:"FIRECRACKER_BRANCH"`
	RDMARepo          string `json:"RDMA_REPO"`
	RDMABranch        string `json:"RDMA_BRANCH"`
	ClusterMode       string `json:"CLUSTER_MODE"`
	PodsPerNode       int    `json:"PODS_PER_NODE"`
	DeployPrometheus  bool   `json:"DEPLOY_PROMETHEUS"`
	DeployMinio       bool   `json:"DEPLOY_MINIO"`
	DeployRDMA        bool   `json:"DEPLOY_RDMA"`
}

type MinioConfig struct {
	HelmDownloadUrl string `json:"HelmDownloadUrl"`
	MinIOVersion    string `json:"MinIOVersion"`
	MinIOValuePath  string `json:"MinIOValuePath"`
	MinIOCClientUrl string `json:"MinIOCClientUrl"`
}

type PrometheusConfig struct {
	MetricsServerVersion    string `json:"MetricsServerVersion"`
	PromChartVersion        string `json:"PromChartVersion"`
	PushgatewayChartVersion string `json:"PushgatewayChartVersion"`
	PromValuePath           string `json:"PromValuePath"`
	KnativePromURL          string `json:"KnativePromURL"`
}

func CommonConfigSetup(configDir string, configName string) (*InvitroSetupConfig, error) {
	// Load Configurations
	_, extNodeSetup, err := GetNodeSetup(configDir, configName)
	if err != nil {
		utils.FatalPrintf("Failed to get node setup config: %v\n", err)
		return nil, err
	}

	setupCfg, err := GetSetupJSON(configDir)
	if err != nil {
		utils.FatalPrintf("Failed to get setup config: %v\n", err)
		return nil, err
	}

	minioConfig, err := GetMinioConfig(configDir)
	if err != nil {
		utils.FatalPrintf("Failed to get MinIO config: %v\n", err)
		return nil, err
	}

	promConfig, err := GetPromConfig(configDir)
	if err != nil {
		utils.FatalPrintf("Failed to get Prometheus config: %v\n", err)
		return nil, err
	}

	masterNode := extNodeSetup.NodeSetup.MasterNode[0]
	loaderNode := extNodeSetup.NodeSetup.LoaderNode[0]
	workerNodes := extNodeSetup.NodeSetup.WorkerNode
	minioOperatorNodes := extNodeSetup.NodeSetup.MinioOperatorNode
	minioTenantNodes := extNodeSetup.NodeSetup.MinioTenantNode
	allNodes := append([]string{masterNode}, workerNodes...)

	return &InvitroSetupConfig{
		MasterNode:         masterNode,
		LoaderNode:         loaderNode,
		WorkerNodes:        workerNodes,
		MinioOperatorNodes: minioOperatorNodes,
		MinioTenantNodes:   minioTenantNodes,
		AllNodes:           allNodes,
		SetupCfg:           setupCfg,
		MinioConfig:        minioConfig,
		PromConfig:         promConfig,
	}, nil
}

func GetNodeSetup(path string, configName string) (*NodeSetup, *NodeSetup, error) {
	configPath := filepath.Join(path, configName)
	configFile, err := os.ReadFile(configPath)
	if err != nil {
		return nil, nil, err
	}

	var intNodeSetup NodeSetup
	var extlNodeSetup NodeSetup
	err = json.Unmarshal(configFile, &intNodeSetup)
	if err != nil {
		return nil, nil, err
	}
	if err := validateNodeSetup(&intNodeSetup); err != nil {
		return nil, nil, fmt.Errorf("invalid node setup %q: %w", configPath, err)
	}

	// Map internal IPs to real world URLs
	ipToURL := mapNodeURLs(&intNodeSetup)

	extlNodeSetup.NodeSetup.MasterNode = swapIPs(intNodeSetup.NodeSetup.MasterNode, ipToURL)
	extlNodeSetup.NodeSetup.LoaderNode = swapIPs(intNodeSetup.NodeSetup.LoaderNode, ipToURL)
	extlNodeSetup.NodeSetup.WorkerNode = swapIPs(intNodeSetup.NodeSetup.WorkerNode, ipToURL)
	extlNodeSetup.NodeSetup.MinioOperatorNode = swapIPs(intNodeSetup.NodeSetup.MinioOperatorNode, ipToURL)
	extlNodeSetup.NodeSetup.MinioTenantNode = swapIPs(intNodeSetup.NodeSetup.MinioTenantNode, ipToURL)

	extlNodeSetup.NodeLabel = make(map[string][]string)
	for k, v := range intNodeSetup.NodeLabel {
		extlNodeSetup.NodeLabel[k] = swapIPs(v, ipToURL)
	}

	return &intNodeSetup, &extlNodeSetup, nil
}

func validateNodeSetup(nodeSetup *NodeSetup) error {
	roles := nodeSetup.NodeSetup
	if len(roles.MasterNode) != 1 {
		return fmt.Errorf("NODE_SETUP.MASTER_NODE must contain exactly one node")
	}
	if len(roles.LoaderNode) != 1 {
		return fmt.Errorf("NODE_SETUP.LOADER_NODE must contain exactly one node")
	}
	if len(roles.WorkerNode) == 0 || len(roles.MinioTenantNode) == 0 {
		return fmt.Errorf("NODE_SETUP.WORKER_NODE and NODE_SETUP.MINIO_TENANT_NODE must be non-empty")
	}
	if len(roles.MinioOperatorNode) == 0 {
		return fmt.Errorf("NODE_SETUP.MINIO_OPERATOR_NODE must be non-empty")
	}
	if len(nodeSetup.NodeURL) == 0 {
		return fmt.Errorf("NODE_URL must be non-empty")
	}
	seenURLs := make(map[string]struct{}, len(nodeSetup.NodeURL))
	for _, url := range nodeSetup.NodeURL {
		if url == "" {
			return fmt.Errorf("NODE_URL must not contain empty entries")
		}
		if _, exists := seenURLs[url]; exists {
			return fmt.Errorf("NODE_URL contains duplicate %q", url)
		}
		seenURLs[url] = struct{}{}
	}
	knownIPs := make(map[string]struct{}, len(nodeSetup.NodeURL))
	for i := range nodeSetup.NodeURL {
		knownIPs["10.0.1."+strconv.Itoa(i+1)] = struct{}{}
	}
	for name, nodes := range map[string][]string{
		"MASTER_NODE": roles.MasterNode, "LOADER_NODE": roles.LoaderNode,
		"WORKER_NODE": roles.WorkerNode, "MINIO_OPERATOR_NODE": roles.MinioOperatorNode,
		"MINIO_TENANT_NODE": roles.MinioTenantNode,
	} {
		for _, node := range nodes {
			if _, ok := knownIPs[node]; !ok {
				return fmt.Errorf("NODE_SETUP.%s references unmapped node %q", name, node)
			}
		}
	}
	requiredLabels := []string{
		"loader-nodetype=master", "loader-nodetype=monitoring",
		"loader-nodetype=worker", "minio-type=operator", "minio-type=tenant",
	}
	for _, label := range requiredLabels {
		nodes, ok := nodeSetup.NodeLabel[label]
		if !ok || len(nodes) == 0 {
			return fmt.Errorf("NODE_LABEL[%q] must be present and non-empty", label)
		}
		for _, node := range nodes {
			if _, ok := knownIPs[node]; !ok {
				return fmt.Errorf("NODE_LABEL[%q] references unmapped node %q", label, node)
			}
		}
	}
	workers := make(map[string]struct{}, len(nodeSetup.NodeLabel["loader-nodetype=worker"]))
	for _, node := range nodeSetup.NodeLabel["loader-nodetype=worker"] {
		workers[node] = struct{}{}
	}
	for _, node := range nodeSetup.NodeLabel["minio-type=tenant"] {
		if _, overlaps := workers[node]; overlaps {
			return fmt.Errorf("worker and MinIO tenant labels overlap on %q", node)
		}
	}
	return nil
}

func GetSetupJSON(path string) (*SetupConfig, error) {
	configPath := filepath.Join(path, "setup.json")
	configFile, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	var setupConfig SetupConfig
	err = json.Unmarshal(configFile, &setupConfig)
	if err != nil {
		return nil, err
	}
	if err := validateSetupConfig(&setupConfig); err != nil {
		return nil, fmt.Errorf("invalid setup config %q: %w", configPath, err)
	}

	return &setupConfig, nil
}

func validateSetupConfig(config *SetupConfig) error {
	required := map[string]string{
		"VHIVE_REPO": config.HiveRepo, "VHIVE_BRANCH": config.HiveBranch,
		"LOADER_REPO": config.LoaderRepo, "LOADER_BRANCH": config.LoaderBranch,
		"KHALA_REPO": config.KhalaRepo, "KHALA_BRANCH": config.KhalaBranch,
		"FIRECRACKER_REPO": config.FirecrackerRepo, "FIRECRACKER_BRANCH": config.FirecrackerBranch,
	}
	if config.DeployRDMA {
		required["RDMA_REPO"] = config.RDMARepo
		required["RDMA_BRANCH"] = config.RDMABranch
	}
	for key, value := range required {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s must be non-empty", key)
		}
	}
	return nil
}

func GetMinioConfig(path string) (*MinioConfig, error) {
	configPath := filepath.Join(path, "minio/minio_config.json")
	configFile, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	var minioConfig MinioConfig
	err = json.Unmarshal(configFile, &minioConfig)
	if err != nil {
		return nil, err
	}

	return &minioConfig, nil
}

func GetPromConfig(path string) (*PrometheusConfig, error) {
	configPath := filepath.Join(path, "prometheus/prom_config.json")
	configFile, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	var promConfig PrometheusConfig
	err = json.Unmarshal(configFile, &promConfig)
	if err != nil {
		return nil, err
	}

	return &promConfig, nil
}

func mapNodeURLs(nodeSetup *NodeSetup) map[string]string {
	mapping := make(map[string]string)
	for i, url := range nodeSetup.NodeURL {
		ip := "10.0.1." + strconv.Itoa(i+1)
		mapping[ip] = url
	}
	return mapping
}

func swapIPs(nodes []string, ipToURL map[string]string) []string {
	swapped := make([]string, len(nodes))
	for i, ip := range nodes {
		if url, ok := ipToURL[ip]; ok {
			swapped[i] = url
		} else {
			swapped[i] = ip
		}
	}
	return swapped
}
