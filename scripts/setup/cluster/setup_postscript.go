package cluster

import (
	loaderUtils "github.com/vhive-serverless/loader/scripts/setup/utils"
)

func applyPostSetupConfigurations(masterNode string) error {

	_, err := loaderUtils.ServerExec(masterNode, "curl -sS https://webi.sh/k9s | sh")
	if err != nil {
		return err
	}

	return nil
}
