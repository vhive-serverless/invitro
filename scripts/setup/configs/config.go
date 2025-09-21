package configs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
)

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
	HiveRepo         string `json:"VHIVE_REPO"`
	HiveBranch       string `json:"VHIVE_BRANCH"`
	LoaderRepo       string `json:"LOADER_REPO"`
	LoaderBranch     string `json:"LOADER_BRANCH"`
	KhalaRepo        string `json:"KHALA_REPO"`
	KhalaBranch      string `json:"KHALA_BRANCH"`
	ClusterMode      string `json:"CLUSTER_MODE"`
	PodsPerNode      int    `json:"PODS_PER_NODE"`
	DeployPrometheus bool   `json:"DEPLOY_PROMETHEUS"`
	DeployMinio      bool   `json:"DEPLOY_MINIO"`
}

type MinioConfig struct {
	HelmDownloadUrl string `json:"HelmDownloadUrl"`
	MinIOVersion    string `json:"MinIOVersion"`
	MinIOValuePath  string `json:"MinIOValuePath"`
	MinIOCClientUrl string `json:"MinIOCClientUrl"`
}

type PrometheusConfig struct {
	MetricsServerVersion string `json:"MetricsServerVersion"`
	PromChartVersion     string `json:"PromChartVersion"`
	PromValuePath        string `json:"PromValuePath"`
	KnativePromURL       string `json:"KnativePromURL"`
}

func GetNodeSetup(path string) (*NodeSetup, *NodeSetup, error) {
	configPath := filepath.Join(path, "node_setup.json")
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

	return &setupConfig, nil
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
