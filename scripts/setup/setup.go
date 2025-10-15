package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/vhive-serverless/loader/scripts/setup/cluster"
	"github.com/vhive-serverless/vHive/scripts/utils"
)

var (
	Setup      = flag.String("setup-type", "create_multinode_cluster", "Type of setup to perform")
	configName = flag.String("config", "node_setup.json", "Configuration file name")
)

func main() {
	flag.Parse()

	availableCmds := []string{
		"create_multinode_cluster",
	}

	switch *Setup {
	case "create_multinode_cluster":
		cluster.CreateMultiNodeSetup("configs", *configName)
		// Call the function to create a multinode cluster

	default:
		utils.FatalPrintf("Invalid subcommand --> %s! Available subcommands list: \n", *Setup)
		for _, subCmd := range availableCmds {
			fmt.Printf("%s\n", subCmd)
		}
		os.Exit(1)
	}
}
