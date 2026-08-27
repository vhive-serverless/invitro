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
