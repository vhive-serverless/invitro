package eval

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
)

type Profile string

const (
	Profile4  Profile = "4-node"
	Profile10 Profile = "10-node"
	Profile14 Profile = "14-node"
	Profile18 Profile = "18-node"
)

type Config struct {
	Profile          Profile
	TopologyConfig   string
	MinioEndpoint    string
	ResultRoot       string
	CampaignManifest string
	Freeze           bool
	DryRun           bool
}

func AddFlags(fs *flag.FlagSet, cfg *Config) {
	fs.Var((*profileValue)(&cfg.Profile), "profile", "topology profile (4-node, 10-node, 14-node, or 18-node)")
	fs.StringVar(&cfg.TopologyConfig, "topology-config", "", "setup topology JSON")
	fs.StringVar(&cfg.MinioEndpoint, "minio-endpoint", "", "Kubernetes-hosted MinIO URL")
	fs.StringVar(&cfg.ResultRoot, "result-root", "", "result directory")
	fs.StringVar(&cfg.CampaignManifest, "campaign-manifest", "", "campaign manifest path")
	fs.BoolVar(&cfg.Freeze, "freeze", false, "create a campaign manifest after preflight")
	fs.BoolVar(&cfg.DryRun, "dry-run", false, "print checks without executing them")
}

type profileValue Profile

func (p *profileValue) String() string     { return string(*p) }
func (p *profileValue) Set(v string) error { *p = profileValue(v); return nil }

type Setup struct {
	NodeSetup struct {
		Master   []string `json:"MASTER_NODE"`
		Loader   []string `json:"LOADER_NODE"`
		Worker   []string `json:"WORKER_NODE"`
		Operator []string `json:"MINIO_OPERATOR_NODE"`
		Tenant   []string `json:"MINIO_TENANT_NODE"`
	} `json:"NODE_SETUP"`
	NodeLabel map[string][]string `json:"NODE_LABEL"`
	NodeURL   []string            `json:"NODE_URL"`
}

func LoadSetup(path string) (Setup, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Setup{}, err
	}
	var setup Setup
	if err := json.Unmarshal(data, &setup); err != nil {
		return Setup{}, err
	}
	return setup, nil
}

type Counts struct{ Master, Loader, Worker, Tenant int }

func ExpectedCounts(profile Profile) (Counts, error) {
	switch profile {
	case Profile4:
		return Counts{1, 1, 1, 1}, nil
	case Profile10:
		return Counts{1, 1, 4, 4}, nil
	case Profile14:
		return Counts{1, 1, 6, 6}, nil
	case Profile18:
		return Counts{1, 1, 8, 8}, nil
	default:
		return Counts{}, fmt.Errorf("unsupported profile %q", profile)
	}
}

func (s Setup) URLForIP(ip string) (string, error) {
	index, err := IPIndex(ip)
	if err != nil || index < 1 || index > len(s.NodeURL) {
		return "", fmt.Errorf("no NODE_URL mapping for %q", ip)
	}
	url := strings.TrimSpace(s.NodeURL[index-1])
	if url == "" {
		return "", fmt.Errorf("NODE_URL[%d] is empty", index-1)
	}
	return url, nil
}

func (s Setup) LabeledIPs(label string) []string {
	values := append([]string(nil), s.NodeLabel[label]...)
	sort.Strings(values)
	return values
}
