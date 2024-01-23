package driver

import (
	log "github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/pkg/common"
	"sync"
	"sync/atomic"
)

func DeployFunctionsAWSLambda(functions []*common.Function) {
	provider := "aws"

	functionGroups := separateFunctions(functions)

	// Use goroutines to create multiple serverless.yml files, deploy functions in parallel, and ensure all finishes
	var wg sync.WaitGroup
	var counter uint64 = 0

	for i := 0; i < len(functionGroups); i++ {
		wg.Add(1)
		go func(functionGroup []*common.Function, index int) {
			defer wg.Done()

			// Create serverless.yml file
			serverless := Serverless{}
			serverless.CreateHeader(index, provider)
			serverless.AddPackagePattern("!**")
			serverless.AddPackagePattern("./server/trace-func-go/aws/trace_func")

			for i := 0; i < len(functionGroup); i++ {
				serverless.AddFunctionConfig(functionGroup[i], provider)
			}

			serverless.CreateServerlessConfigFile(index)

			// Deploy serverless functions and update the function endpoints
			functionToURLMapping := DeployServerless(index)

			if functionToURLMapping == nil {
				log.Fatalf("Failed to deploy serverless.yml file %d", index)
			} else {
				atomic.AddUint64(&counter, 1)
				for i := 0; i < len(functionGroup); i++ {
					functionGroup[i].Endpoint = functionToURLMapping[functionGroup[i].Name]
					log.Debugf("Function %s set to %s", functionGroup[i].Name, functionGroup[i].Endpoint)
				}
			}
		}(functionGroups[i], i)
	}

	wg.Wait()

	log.Debugf("Deployed %d out of %d serverless.yml files", counter, len(functionGroups))
}

func CleanAWSLambda(functions []*common.Function) {
	functionGroups := separateFunctions(functions)

	// Use goroutines to delete multiple serverless.yml files in parallel
	var wg sync.WaitGroup
	var counter uint64 = 0

	for i := 0; i < len(functionGroups); i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			deleted := CleanServerless(index)
			if deleted {
				atomic.AddUint64(&counter, 1)
			}
		}(i)
	}

	wg.Wait()

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
