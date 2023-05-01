package driver

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"

	"github.com/eth-easl/loader/pkg/common"
	log "github.com/sirupsen/logrus"
)

const actionName = "tester"

func DeployFunctionsOpenWhisk(functions []*common.Function) {
	cmd := exec.Command("wsk", "-i", "property", "get", "--apihost")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		log.Fatal("Unable to read OpenWhisk API host data.")
	}

	result := strings.Split(out.String(), "\t")
	endpoint := strings.TrimSpace(result[len(result)-1])

	const actionLocation = "./pkg/workload/openwhisk/workload_openwhisk.go"
	cmd = exec.Command("wsk", "-i", "action", "create", actionName, actionLocation, "--web", "true")
	err = cmd.Run()
	if err != nil {
		log.Fatal("Unable to create OpenWhisk action.")
	}

	functionURL := fmt.Sprintf("https://%s/api/v1/web/guest/default/%s", endpoint, actionName)

	for i := 0; i < len(functions); i++ {
		functions[i].Endpoint = functionURL
	}
}

func CleanOpenWhisk() {
	cmd := exec.Command("wsk", "-i", "action", "delete", actionName)
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		log.Debugf("Unable to delete OpenWhisk action.")
	}
}
