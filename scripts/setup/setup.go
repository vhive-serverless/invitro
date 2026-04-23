package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/vhive-serverless/loader/scripts/setup/cluster"
	loaderUtils "github.com/vhive-serverless/loader/scripts/setup/utils"
	"github.com/vhive-serverless/vHive/scripts/utils"
)

var (
	Setup      = flag.String("setup-type", "create_multinode_cluster", "Type of setup to perform")
	configName = flag.String("config", "node_setup.json", "Configuration file name")
	configDir  = flag.String("config-dir", "", "Path to setup config directory")
)

func main() {
	flag.Parse()

	availableCmds := []string{
		"create_multinode_cluster",
	}

	dir, err := loaderUtils.ResolveConfigDir(*configDir, *configName)
	if err != nil {
		utils.FatalPrintf("%v\n", err)
		os.Exit(1)
	}

	switch *Setup {
	case "create_multinode_cluster":
		selectedConfigName := *configName
		if flag.NArg() > 0 {
			selectedConfigName, err = loaderUtils.CreateTempNodeSetup(dir, flag.Args())
			if err != nil {
				utils.FatalPrintf("Failed to create temporary node setup config: %v\n", err)
				os.Exit(1)
			}
			defer os.Remove(filepath.Join(dir, selectedConfigName))
		}

		err := cluster.CreateMultiNodeSetup(dir, selectedConfigName)
		if err != nil {
			utils.FatalPrintf("Failed to create multinode cluster: %v\n", err)
			os.Exit(1)
		}
		// Call the function to create a multinode cluster

	default:
		utils.FatalPrintf("Invalid subcommand --> %s! Available subcommands list: \n", *Setup)
		for _, subCmd := range availableCmds {
			fmt.Printf("%s\n", subCmd)
		}
		os.Exit(1)
	}
}
