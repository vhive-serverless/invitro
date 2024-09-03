package deployment

import (
	"bytes"
	"fmt"
	"github.com/vhive-serverless/loader/pkg/config"
	"os/exec"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/pkg/common"
)

type openWhiskDeployer struct {
	functions []*common.Function
}

func newOpenWhiskDeployer() *openWhiskDeployer {
	return &openWhiskDeployer{}
}

func (owd *openWhiskDeployer) Deploy(cfg *config.Configuration) {
	owd.functions = cfg.Functions

	cmd := exec.Command("wsk", "-i", "property", "get", "--apihost")

	var out bytes.Buffer
	cmd.Stdout = &out

	err := cmd.Run()
	if err != nil {
		log.Fatalf("Unable to read OpenWhisk API host data - %s", err)
	}
	result := strings.Split(out.String(), "\t")
	endpoint := strings.TrimSpace(result[len(result)-1])

	const actionLocation = "./pkg/workload/openwhisk/workload_openwhisk.go"

	for i := 0; i < len(owd.functions); i++ {
		cmd = exec.Command("wsk", "-i", "action", "create", owd.functions[i].Name, actionLocation, "--kind", "go:1.17", "--web", "true")

		err = cmd.Run()
		if err != nil {
			log.Fatalf("Unable to create OpenWhisk action for function %s - %s", owd.functions[i].Name, err)
		}

		owd.functions[i].Endpoint = fmt.Sprintf("https://%s/api/v1/web/guest/default/%s", endpoint, owd.functions[i].Name)
	}
}

func (owd *openWhiskDeployer) Clean() {
	for i := 0; i < len(owd.functions); i++ {
		// TODO: check if there is a command such as "... delete --all"
		cmd := exec.Command("wsk", "-i", "action", "delete", owd.functions[i].Name)

		var out bytes.Buffer
		cmd.Stdout = &out

		err := cmd.Run()
		if err != nil {
			log.Debugf("Unable to delete OpenWhisk action for function %s - %s", owd.functions[i].Name, err)
		}
	}
}
