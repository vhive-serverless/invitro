package cluster

import (
	"errors"
	"fmt"
	"sync"

	"github.com/vhive-serverless/loader/scripts/setup/configs"
	loaderUtils "github.com/vhive-serverless/loader/scripts/setup/utils"
	"github.com/vhive-serverless/vHive/scripts/utils"
)

func setupRDMA(cfg *configs.SetupConfig, tenantNodes []string) error {
	var wg sync.WaitGroup
	var setupErrors []error
	var errorsMu sync.Mutex
	addError := func(err error) {
		if err != nil {
			errorsMu.Lock()
			setupErrors = append(setupErrors, err)
			errorsMu.Unlock()
		}
	}

	commandList := []string{
		"sudo apt-get update",
		fmt.Sprintf("git clone --branch %s --single-branch %s", cfg.RDMABranch, cfg.RDMARepo),
		"source /etc/profile && cd rdma-demo && make install-deps && make build-all",
	}

	for _, node := range tenantNodes {
		wg.Add(1)
		go func(node string) {
			defer wg.Done()
			utils.WaitPrintf("Setting up RDMA on node: %s\n", node)
			for _, cmd := range commandList {
				_, err := loaderUtils.ServerExec(node, cmd)
				if !utils.CheckErrorWithMsg(err, "Failed to execute %s on node %s: %v\n", cmd, node, err) {
					addError(fmt.Errorf("execute %q on %s: %w", cmd, node, err))
					return
				}
			}
		}(node)
	}
	wg.Wait()
	return errors.Join(setupErrors...)
}

func rdmaPayloadProvisionCommands() []string {
	return []string{
		"mkdir -p ~/rdma-demo/assets/nexus-benchmark-payload/input_payload ~/rdma-demo/assets/nexus-benchmark-payload/test ~/rdma-demo/assets/synthetic-payload-input",
		"rsync -a ~/khala/assets/nexus-benchmark-payload/input_payload/ ~/rdma-demo/assets/nexus-benchmark-payload/input_payload/",
		"rsync -a ~/khala/assets/nexus-benchmark-payload/test/ ~/rdma-demo/assets/nexus-benchmark-payload/test/",
		"rsync -a ~/khala/assets/synthetic-payload/ ~/rdma-demo/assets/synthetic-payload-input/",
		"test \"$(sha256sum ~/khala/assets/nexus-benchmark-payload/input_payload/mapper_scaled/part-00000.csv | cut -d' ' -f1)\" = \"$(sha256sum ~/rdma-demo/assets/nexus-benchmark-payload/input_payload/mapper_scaled/part-00000.csv | cut -d' ' -f1)\"",
	}
}

// setupRDMAPayloads runs after setupKhala has distributed the canonical Khala
// assets to every Kubernetes worker, including the RDMA tenant nodes. The
// rdma-demo repository intentionally ignores assets/**/*, so cloning and
// building it cannot populate the server's payload root.
func setupRDMAPayloads(tenantNodes []string) error {
	var wg sync.WaitGroup
	var setupErrors []error
	var errorsMu sync.Mutex
	for _, node := range tenantNodes {
		wg.Add(1)
		go func(node string) {
			defer wg.Done()
			for _, command := range rdmaPayloadProvisionCommands() {
				if _, err := loaderUtils.ServerExec(node, command); err != nil {
					errorsMu.Lock()
					setupErrors = append(setupErrors, fmt.Errorf("provision RDMA payloads on %s with %q: %w", node, command, err))
					errorsMu.Unlock()
					return
				}
			}
		}(node)
	}
	wg.Wait()
	return errors.Join(setupErrors...)
}
