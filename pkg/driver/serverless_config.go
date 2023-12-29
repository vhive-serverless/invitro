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
	"fmt"
	log "github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/pkg/common"
	"gopkg.in/yaml.v3"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

// Serverless describes the serverless.yml contents.
type Serverless struct {
	Service          string                  `yaml:"service"`
	FrameworkVersion string                  `yaml:"frameworkVersion"`
	Provider         slsProvider             `yaml:"provider"`
	Package          slsPackage              `yaml:"package"`
	Functions        map[string]*slsFunction `yaml:"functions"`
}

type slsProvider struct {
	Name    string `yaml:"name"`
	Runtime string `yaml:"runtime"`
	Stage   string `yaml:"stage"`
	Region  string `yaml:"region"`
}

type slsPackage struct {
	Patterns []string `yaml:"patterns"`
}

type slsFunction struct {
	Handler     string     `yaml:"handler"`
	Description string     `yaml:"description"`
	Name        string     `yaml:"name"`
	Events      []slsEvent `yaml:"events"`
	Timeout     string     `yaml:"timeout"`
}

type slsEvent struct {
	HttpApi slsHttpApi `yaml:"httpApi"`
}

type slsHttpApi struct {
	Path   string `yaml:"path"`
	Method string `yaml:"method"`
}

// CreateHeader sets the fields Service, FrameworkVersion, and Provider
func (s *Serverless) CreateHeader(index int, provider string) {
	s.Service = fmt.Sprintf("loader-%d", index)
	s.FrameworkVersion = "3"
	s.Provider = slsProvider{
		Name:    provider,
		Runtime: "go1.x",
		Stage:   "dev",
		Region:  "us-east-1",
	}
	s.Functions = map[string]*slsFunction{}
}

func stringContains(s []string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}
	return false
}

// AddPackagePattern adds a string pattern to Package.Pattern as long as such a pattern does not already exist in Package.Pattern
func (s *Serverless) AddPackagePattern(pattern string) {
	if !stringContains(s.Package.Patterns, pattern) {
		s.Package.Patterns = append(s.Package.Patterns, pattern)
	}
}

// AddFunctionConfig adds the function configuration for serverless.com deployment
func (s *Serverless) AddFunctionConfig(function *common.Function, provider string) {

	events := []slsEvent{{slsHttpApi{Path: "/" + function.Name, Method: "GET"}}}

	var handler string
	var timeout string
	switch provider {
	case "aws":
		handler = "server/trace-func-go/aws/trace_func"
		timeout = "29"
	default:
		log.Fatalf("AddFunctionConfig could not recognize provider %s", provider)
	}

	f := &slsFunction{Handler: handler, Name: function.Name, Events: events, Timeout: timeout}
	s.Functions[function.Name] = f
}

// CreateServerlessConfigFile dumps the contents of the Serverless struct into a yml file (serverless-<index>.yml)
func (s *Serverless) CreateServerlessConfigFile(index int) {
	data, err := yaml.Marshal(&s)
	if err != nil {
		log.Fatal(err)
	}

	err = os.WriteFile(fmt.Sprintf("./serverless-%d.yml", index), data, os.FileMode(0644))

	if err != nil {
		log.Fatal(err)
	}
}

// DeployServerless deploys the functions defined in the serverless.com file and returns a map from function name to URL
func DeployServerless(index int) map[string]string {
	slsDeployCmd := exec.Command("sls", "deploy", "--config", fmt.Sprintf("./serverless-%d.yml", index))
	stdoutStderr, err := slsDeployCmd.CombinedOutput()
	log.Debug("CMD response: ", string(stdoutStderr))

	// Extract the URLs from the output
	urlPattern := `https://\S+`
	urlRegex := regexp.MustCompile(urlPattern)
	urlMatches := urlRegex.FindAllStringSubmatch(string(stdoutStderr), -1)

	// Map the function names (endpoints) to the URLs
	functionToURL := make(map[string]string)
	for i := 0; i < len(urlMatches); i++ {
		functionToURL[strings.SplitN(urlMatches[i][0], "/", 4)[3]] = urlMatches[i][0]
	}

	if err != nil {
		log.Fatalf("Failed to deploy serverless-%d.yml: %v\n%s", index, err, stdoutStderr)
		return nil
	}

	log.Debugf("Deployed serverless-%d.yml", index)
	return functionToURL
}

// CleanServerless removes the deployed service and deletes the serverless-<index>.yml file
func CleanServerless(index int) bool {
	slsRemoveCmd := exec.Command("sls", "remove", "--config", fmt.Sprintf("./serverless-%d.yml", index))
	stdoutStderr, err := slsRemoveCmd.CombinedOutput()
	log.Debug("CMD response: ", string(stdoutStderr))

	if err != nil {
		log.Warnf("Failed to undeploy serverless-%d.yml: %v\n%s", index, err, stdoutStderr)
		return false
	}

	slsRemoveCmd = exec.Command("rm", "-f", fmt.Sprintf("./serverless-%d.yml", index))
	stdoutStderr, err = slsRemoveCmd.CombinedOutput()

	if err != nil {
		log.Warnf("Failed to delete serverless-%d.yml: %v\n%s", index, err, stdoutStderr)
		return false
	}

	log.Debugf("Undeployed and deleted serverless-%d.yml", index)
	return true
}
