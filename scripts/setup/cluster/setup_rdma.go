package cluster

import (
	"sync"

	loaderUtils "github.com/vhive-serverless/loader/scripts/setup/utils"
	"github.com/vhive-serverless/vHive/scripts/utils"
)

func setupRDMA(tenantNodes []string) error {
	var wg sync.WaitGroup
	errChan := make(chan error, len(tenantNodes))

	commandList := []string{
		"sudo apt-get update",
		"git clone https://github.com/hyscale-lab/rdma-demo.git",
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
					errChan <- err
					return
				}
			}
		}(node)
	}
	wg.Wait()
	return nil
}
