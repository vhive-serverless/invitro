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

package driver

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/pkg/common"
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
