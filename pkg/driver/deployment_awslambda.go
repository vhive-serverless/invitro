package driver

import (
	log "github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/pkg/common"
	"os/exec"
	"sync"
	"sync/atomic"
)

func DeployFunctionsAWSLambda(functions []*common.Function) {
	provider := "aws"

	// Copy built Go binary to avoid nested directories during deployment (https://aws.amazon.com/fr/blogs/compute/migrating-aws-lambda-functions-from-the-go1-x-runtime-to-the-custom-runtime-on-amazon-linux-2/)
	// Go binary is built in Linux by: CGO_ENABLED=1 GOARCH=amd64 GOGCCFLAGS=-m64 GOOS=linux go build -o ./server/trace-func-go/aws/bootstrap ./server/trace-func-go/aws/trace_func.go
	copyBinaryCmd := exec.Command("cp", "./server/trace-func-go/aws/bootstrap", "./bootstrap")
	err := copyBinaryCmd.Run()
	if err != nil {
		log.Fatalf("Failed to copy Go binary: %s", err)
	}

	functionGroups := separateFunctions(functions)

	// Use goroutines to create multiple serverless.yml files, deploy functions in parallel, and ensure all finishes
	// However, due to CPU and memory constraints, we will only deploy 2 serverless.yml files in parallel and wait for them to finish before deploying the next 2
	var wg sync.WaitGroup
	var counter uint64 = 0
	parallelDeployment := 2

	for i := 0; i < len(functionGroups); {
		for parallelIndex := 0; parallelIndex < parallelDeployment; parallelIndex++ {
			if i < len(functionGroups) {
				wg.Add(1)
				go func(functionGroup []*common.Function, index int) {
					defer wg.Done()
					log.Debugf("Creating serverless-%d.yml", index)

					// Create serverless.yml file
					serverless := Serverless{}
					serverless.CreateHeader(index, provider)
					serverless.AddPackagePattern("!**")
					serverless.AddPackagePattern("bootstrap")

					for i := 0; i < len(functionGroup); i++ {
						serverless.AddFunctionConfig(functionGroup[i], provider)
					}

					serverless.CreateServerlessConfigFile(index)

					log.Debugf("Deploying serverless-%d.yml", index)
					// Deploy serverless functions and update the function endpoints
					functionToURLMapping := DeployServerless(index)

					if functionToURLMapping == nil {
						log.Fatalf("Failed to deploy serverless-%d.yml", index)
					} else {
						atomic.AddUint64(&counter, 1)
						for i := 0; i < len(functionGroup); i++ {
							functionGroup[i].Endpoint = functionToURLMapping[i]
							log.Debugf("Function %s set to %s", functionGroup[i].Name, functionGroup[i].Endpoint)
						}
					}
				}(functionGroups[i], i)
				i += 1
			}
		}
		wg.Wait()
	}

	log.Debugf("Deployed %d out of %d serverless.yml files", counter, len(functionGroups))
}

func CleanAWSLambda(functions []*common.Function) {
	// Delete bootstrap binary
	deleteBootstrapCmd := exec.Command("rm", "./bootstrap")
	err := deleteBootstrapCmd.Run()
	if err != nil {
		log.Fatalf("Failed to delete bootstrap binary: %s", err)
	}

	functionGroups := separateFunctions(functions)

	// Use goroutines to delete multiple serverless.yml files in parallel
	// However, due to CPU and memory constraints, we will only undeploy 2 serverless.yml files in parallel and wait for them to finish before undeploying the next 2
	var wg sync.WaitGroup
	var counter uint64 = 0
	parallelDeployment := 2

	for i := 0; i < len(functionGroups); {
		for parallelIndex := 0; parallelIndex < parallelDeployment; parallelIndex++ {
			if i < len(functionGroups) {
				wg.Add(1)
				go func(index int) {
					defer wg.Done()
					log.Debugf("Undeploying serverless-%d.yml", index)
					deleted := CleanServerless(index)
					if deleted {
						atomic.AddUint64(&counter, 1)
					}
				}(i)
				i += 1
			}
		}
		wg.Wait()
	}

	log.Debugf("Deleted %d out of %d serverless.yml files", counter, len(functionGroups))
}

// separateFunctions splits functions into groups of 70 due to AWS CloudFormation template resource limit (500 resources per template) and IAM maximum policy size (10240 bytes)
func separateFunctions(functions []*common.Function) [][]*common.Function {
	var functionGroups [][]*common.Function
	groupSize := 70

	for i := 0; i < len(functions); i += groupSize {
		end := i + groupSize
		if end > len(functions) {
			end = len(functions)
		}
		functionGroups = append(functionGroups, functions[i:end])
	}

	return functionGroups
}
