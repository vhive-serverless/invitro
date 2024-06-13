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
	log "github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/pkg/common"
	"gopkg.in/yaml.v3"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

// awsServerless describes the serverless.yml contents.
type awsServerless struct {
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
}

type slsEvent struct {
	HttpApi slsHttpApi `yaml:"httpApi"`
}

type slsHttpApi struct {
	Path   string `yaml:"path"`
	Method string `yaml:"method"`
}

// CreateHeader sets the fields Service, FrameworkVersion, and Provider
func (s *awsServerless) CreateHeader(provider string) {
	s.Service = "loader"
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
func (s *awsServerless) AddPackagePattern(pattern string) {
	if !stringContains(s.Package.Patterns, pattern) {
		s.Package.Patterns = append(s.Package.Patterns, pattern)
	}
}

// AddFunctionConfig adds the function configuration for serverless.com deployment
func (s *awsServerless) AddFunctionConfig(function *common.Function, provider string) {

	events := []slsEvent{{slsHttpApi{Path: "/" + function.Name, Method: "GET"}}}

	var handler string
	switch provider {
	case "aws":
		handler = "server/trace-func-go/aws/trace_func"
	default:
		log.Fatalf("AddFunctionConfig could not recognize provider %s", provider)
	}

	f := &slsFunction{Handler: handler, Name: function.Name, Events: events}
	s.Functions[function.Name] = f
}

// CreateServerlessConfigFile dumps the contents of the awsServerless struct into a yml file.
func (s *awsServerless) CreateServerlessConfigFile() {
	data, err := yaml.Marshal(&s)
	if err != nil {
		log.Fatal(err)
	}

	err = os.WriteFile("./serverless.yml", data, os.FileMode(0644))

	if err != nil {
		log.Fatal(err)
	}
}

// DeployServerless deploys the functions defined in the serverless.com file and returns a map from function name to URL
func DeployServerless() map[string]string {
	slsDeployCmd := exec.Command("sls", "deploy")
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
		log.Warnf("Failed to deploy serverless.yml: %v\n%s", err, stdoutStderr)
		return nil
	}
	return functionToURL
}

// CleanServerless removes the service defined in serverless.yml
func CleanServerless() {
	slsRemoveCmd := exec.Command("sls", "remove")
	stdoutStderr, err := slsRemoveCmd.CombinedOutput()
	log.Debug("CMD response: ", string(stdoutStderr))

	if err != nil {
		log.Warnf("Failed to deploy serverless.yml: %v\n%s", err, stdoutStderr)
	}
}
