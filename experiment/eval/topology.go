package eval

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

func ValidateSetup(setup Setup, profile Profile) error {
	expected, err := ExpectedCounts(profile)
	if err != nil {
		return err
	}
	if len(setup.NodeSetup.Master) != 1 || len(setup.NodeSetup.Loader) != 1 {
		return fmt.Errorf("setup requires exactly one master and loader")
	}
	total := 2 + expected.Worker + expected.Tenant
	if len(setup.NodeURL) != total {
		return fmt.Errorf("NODE_URL has %d nodes, want %d for %s", len(setup.NodeURL), total, profile)
	}
	if len(setup.NodeSetup.Worker) != total-1 {
		return fmt.Errorf("NODE_SETUP.WORKER_NODE has %d nodes, want every non-master node (%d)", len(setup.NodeSetup.Worker), total-1)
	}
	known := make(map[string]bool, len(setup.NodeURL))
	seenURLs := make(map[string]bool, len(setup.NodeURL))
	for i, url := range setup.NodeURL {
		url = strings.TrimSpace(url)
		if url == "" {
			return fmt.Errorf("NODE_URL[%d] is empty", i)
		}
		if seenURLs[url] {
			return fmt.Errorf("NODE_URL contains duplicate %q", url)
		}
		seenURLs[url] = true
		known[fmt.Sprintf("10.0.1.%d", i+1)] = true
	}
	for name, nodes := range map[string][]string{"MASTER_NODE": setup.NodeSetup.Master, "LOADER_NODE": setup.NodeSetup.Loader, "WORKER_NODE": setup.NodeSetup.Worker, "MINIO_OPERATOR_NODE": setup.NodeSetup.Operator, "MINIO_TENANT_NODE": setup.NodeSetup.Tenant} {
		for _, node := range nodes {
			if !known[node] {
				return fmt.Errorf("%s references unmapped IP %q", name, node)
			}
		}
	}
	required := map[string]int{"loader-nodetype=master": expected.Master, "loader-nodetype=monitoring": expected.Loader, "loader-nodetype=worker": expected.Worker, "minio-type=tenant": expected.Tenant, "minio-type=operator": 1}
	for label, count := range required {
		nodes, ok := setup.NodeLabel[label]
		if !ok || len(nodes) != count {
			return fmt.Errorf("label %q has %d nodes, want %d", label, len(nodes), count)
		}
		for _, node := range nodes {
			if !known[node] {
				return fmt.Errorf("label %q references unmapped IP %q", label, node)
			}
		}
	}
	workers := map[string]bool{}
	for _, node := range setup.NodeLabel["loader-nodetype=worker"] {
		workers[node] = true
	}
	for _, node := range setup.NodeLabel["minio-type=tenant"] {
		if workers[node] {
			return fmt.Errorf("worker and tenant roles overlap at %q", node)
		}
	}
	for label, setupNodes := range map[string][]string{
		"loader-nodetype=master":     setup.NodeSetup.Master,
		"loader-nodetype=monitoring": setup.NodeSetup.Loader,
		"minio-type=operator":        setup.NodeSetup.Operator,
		"minio-type=tenant":          setup.NodeSetup.Tenant,
	} {
		if !sameStrings(setup.NodeLabel[label], setupNodes) {
			return fmt.Errorf("label %q does not match its NODE_SETUP role", label)
		}
	}
	nonMaster := make(map[string]bool, len(setup.NodeSetup.Worker))
	for _, ip := range setup.NodeSetup.Worker {
		nonMaster[ip] = true
	}
	for _, label := range []string{"loader-nodetype=monitoring", "loader-nodetype=worker", "minio-type=tenant"} {
		for _, ip := range setup.NodeLabel[label] {
			if !nonMaster[ip] {
				return fmt.Errorf("label %q node %q is absent from NODE_SETUP.WORKER_NODE", label, ip)
			}
		}
	}
	return nil
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	a, b := append([]string(nil), left...), append([]string(nil), right...)
	sort.Strings(a)
	sort.Strings(b)
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

type LiveNode struct{ Name, InternalIP, LoaderType, MinioType, Ready string }
type liveNodes struct {
	Items []struct {
		Metadata struct {
			Name   string            `json:"name"`
			Labels map[string]string `json:"labels"`
		} `json:"metadata"`
		Status struct {
			Addresses []struct {
				Type    string `json:"type"`
				Address string `json:"address"`
			} `json:"addresses"`
			Conditions []struct {
				Type   string `json:"type"`
				Status string `json:"status"`
			} `json:"conditions"`
		} `json:"status"`
	} `json:"items"`
}

func ParseLiveNodes(data []byte) ([]LiveNode, error) {
	var raw liveNodes
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	result := make([]LiveNode, 0, len(raw.Items))
	for _, item := range raw.Items {
		n := LiveNode{Name: item.Metadata.Name, LoaderType: item.Metadata.Labels["loader-nodetype"], MinioType: item.Metadata.Labels["minio-type"]}
		for _, a := range item.Status.Addresses {
			if a.Type == "InternalIP" {
				n.InternalIP = a.Address
				break
			}
		}
		for _, c := range item.Status.Conditions {
			if c.Type == "Ready" {
				n.Ready = c.Status
			}
		}
		result = append(result, n)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func ValidateLive(nodes []LiveNode, setup Setup, profile Profile) error {
	if err := ValidateSetup(setup, profile); err != nil {
		return err
	}
	expected, _ := ExpectedCounts(profile)
	if len(nodes) != 2+expected.Worker+expected.Tenant {
		return fmt.Errorf("live node count %d does not match profile %s", len(nodes), profile)
	}
	counts := Counts{}
	liveByIP := make(map[string]LiveNode, len(nodes))
	for _, n := range nodes {
		if n.Ready != "True" {
			return fmt.Errorf("node %q is not Ready", n.Name)
		}
		switch n.LoaderType {
		case "master":
			counts.Master++
		case "monitoring":
			counts.Loader++
		case "worker":
			counts.Worker++
		}
		if n.MinioType == "tenant" {
			counts.Tenant++
		}
		if n.InternalIP == "" || liveByIP[n.InternalIP].Name != "" {
			return fmt.Errorf("missing or duplicate live internal IP %q", n.InternalIP)
		}
		liveByIP[n.InternalIP] = n
	}
	if counts != expected {
		return fmt.Errorf("live role counts %+v, want %+v", counts, expected)
	}
	for label, ips := range setup.NodeLabel {
		for _, ip := range ips {
			n, ok := liveByIP[ip]
			if !ok {
				return fmt.Errorf("configured %s node %s is absent from live cluster", label, ip)
			}
			parts := strings.SplitN(label, "=", 2)
			if len(parts) != 2 {
				continue
			}
			actual := n.LoaderType
			if parts[0] == "minio-type" {
				actual = n.MinioType
			}
			if actual != parts[1] {
				return fmt.Errorf("live node %s label %s=%q, want %q", ip, parts[0], actual, parts[1])
			}
		}
	}
	return nil
}

func KubectlNodes() ([]LiveNode, error) {
	out, err := exec.Command("kubectl", "get", "nodes", "-o", "json").Output()
	if err != nil {
		return nil, err
	}
	return ParseLiveNodes(out)
}

func Pairing(setup Setup) map[string]string {
	result := map[string]string{}
	workers := setup.LabeledIPs("loader-nodetype=worker")
	tenants := setup.LabeledIPs("minio-type=tenant")
	for i, worker := range workers {
		if i < len(tenants) {
			result[worker] = tenants[i]
		}
	}
	return result
}
func IPIndex(ip string) (int, error) {
	parts := strings.Split(ip, ".")
	if len(parts) != 4 || parts[0] != "10" || parts[1] != "0" || parts[2] != "1" {
		return 0, fmt.Errorf("invalid internal IP %q", ip)
	}
	n, err := strconv.Atoi(parts[3])
	return n, err
}
