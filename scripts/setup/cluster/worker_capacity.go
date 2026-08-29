package cluster

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/vhive-serverless/loader/scripts/setup/configs"
	loaderUtils "github.com/vhive-serverless/loader/scripts/setup/utils"
)

const workerNodeLabel = "loader-nodetype=worker"

type workerCapacityList struct {
	Items []struct {
		Metadata struct {
			Name string `json:"name"`
		} `json:"metadata"`
		Status struct {
			Allocatable map[string]string `json:"allocatable"`
			Conditions  []struct {
				Type   string `json:"type"`
				Status string `json:"status"`
			} `json:"conditions"`
		} `json:"status"`
	} `json:"items"`
}

// SetWorkerPodCapacity updates only configured loader-nodetype=worker nodes.
// Calico owns pod CIDR allocation, so this operation deliberately does not
// rewrite Node objects or touch the Knative control plane.
func SetWorkerPodCapacity(configDir, configName string) error {
	_, topology, err := configs.GetNodeSetup(configDir, configName)
	if err != nil {
		return err
	}
	setupCfg, err := configs.GetSetupJSON(configDir)
	if err != nil {
		return err
	}
	if setupCfg.PodsPerNode <= 0 {
		return fmt.Errorf("PODS_PER_NODE must be positive, got %d", setupCfg.PodsPerNode)
	}
	workers := topology.NodeLabel[workerNodeLabel]
	if len(workers) == 0 {
		return fmt.Errorf("topology has no %s nodes", workerNodeLabel)
	}
	master := topology.NodeSetup.MasterNode[0]
	for _, worker := range workers {
		if _, err := loaderUtils.ServerExec(worker, workerMaxPodsCommand(setupCfg.PodsPerNode)); err != nil {
			return fmt.Errorf("set maxPods on worker %s: %w", worker, err)
		}
		if _, err := loaderUtils.ServerExec(worker, "sudo systemctl restart kubelet"); err != nil {
			return fmt.Errorf("restart kubelet on worker %s: %w", worker, err)
		}
	}

	return waitForWorkerCapacity(master, setupCfg.PodsPerNode, len(workers), 2*time.Minute)
}

func workerMaxPodsCommand(target int) string {
	return fmt.Sprintf("sudo sed -i '/^[[:space:]]*maxPods:/d' /var/lib/kubelet/config.yaml && echo \"maxPods: %d\" | sudo tee -a /var/lib/kubelet/config.yaml >/dev/null", target)
}

func waitForWorkerCapacity(master string, target, expectedWorkers int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		output, err := loaderUtils.ServerExec(master, fmt.Sprintf("kubectl get nodes -l %s -o json", workerNodeLabel))
		if err != nil {
			lastErr = fmt.Errorf("query labeled worker capacity: %w", err)
		} else {
			lastErr = validateWorkerCapacity(output, target, expectedWorkers)
			if lastErr == nil {
				return nil
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("worker capacity did not converge within %s: %w", timeout, lastErr)
		}
		time.Sleep(2 * time.Second)
	}
}

func validateWorkerCapacity(data string, target, expectedWorkers int) error {
	var nodes workerCapacityList
	if err := json.Unmarshal([]byte(data), &nodes); err != nil {
		return fmt.Errorf("parse labeled worker capacity: %w", err)
	}
	if len(nodes.Items) != expectedWorkers {
		return fmt.Errorf("found %d labeled worker nodes, expected %d", len(nodes.Items), expectedWorkers)
	}
	for _, node := range nodes.Items {
		ready := false
		for _, condition := range node.Status.Conditions {
			if condition.Type == "Ready" && condition.Status == "True" {
				ready = true
				break
			}
		}
		if !ready {
			return fmt.Errorf("worker %s is not Ready", node.Metadata.Name)
		}
		pods, err := strconv.Atoi(node.Status.Allocatable["pods"])
		if err != nil || pods < target {
			return fmt.Errorf("worker %s allocatable.pods=%q, want >= %d", node.Metadata.Name, node.Status.Allocatable["pods"], target)
		}
	}
	return nil
}
