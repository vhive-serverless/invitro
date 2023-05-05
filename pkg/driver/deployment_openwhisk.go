package driver

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"

	"github.com/eth-easl/loader/pkg/common"
	log "github.com/sirupsen/logrus"
)

func DeployFunctionsOpenWhisk(functions []*common.Function) {
	cmd := exec.Command("wsk", "-i", "property", "get", "--apihost")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		log.Fatalf("Unable to read OpenWhisk API host data - %s", err)
	}

	result := strings.Split(out.String(), "\t")
	endpoint := strings.TrimSpace(result[len(result)-1])

	const actionLocation = "./pkg/workload/openwhisk/workload_openwhisk.zip"

	for i := 0; i < len(functions); i++ {
		cmd = exec.Command("wsk", "-i", "action", "create", functions[i].Name, actionLocation, "--kind", "go:1.17", "--web", "true")

		err = cmd.Run()
		if err != nil {
			log.Fatalf("Unable to create OpenWhisk action for function %s - %s", functions[i].Name, err)
		}

		functions[i].Endpoint = fmt.Sprintf("https://%s/api/v1/web/guest/default/%s", endpoint, functions[i].Name)
	}
}

func CleanOpenWhisk(functions []*common.Function) {
	for i := 0; i < len(functions); i++ {
		cmd := exec.Command("wsk", "-i", "action", "delete", functions[i].Name)
		var out bytes.Buffer
		cmd.Stdout = &out
		err := cmd.Run()
		if err != nil {
			log.Debugf("Unable to delete OpenWhisk action for function %s - %s", functions[i].Name, err)
		}
	}
}
