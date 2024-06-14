/*
 * MIT License
 *
 * Copyright (c) 2023 EASL and the vHive community
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 */

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

type openWhiskDeploymentConfiguration struct {
}

func newOpenWhiskDeployer() *openWhiskDeployer {
	return &openWhiskDeployer{}
}

func newOpenWhiskDeployerConfiguration(_ *config.Configuration) openWhiskDeploymentConfiguration {
	return openWhiskDeploymentConfiguration{}
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
