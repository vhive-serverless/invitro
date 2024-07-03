package deployment

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/pkg/common"
	"github.com/vhive-serverless/loader/pkg/config"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
)

type awsLambdaDeployer struct {
	functions []*common.Function
}

type awsLambdaDeploymentConfiguration struct{}

func newAWSLambdaDeployer() *awsLambdaDeployer {
	return &awsLambdaDeployer{}
}

func (ld *awsLambdaDeployer) Deploy(cfg *config.Configuration) {
	ld.functions = cfg.Functions

	internalAWSDeployment(cfg.Functions)
}

func (ld *awsLambdaDeployer) Clean() {
	CleanAWSLambda(ld.functions)
}

func internalAWSDeployment(functions []*common.Function) {
	const provider = "aws"

	// Check if all required dependencies are installed, verify that AWS account is clean and ready for deployment
	awsAccountId, functionGroups := initAWSLambda(functions, provider)

	// Create all the serverless.yml files
	createSlsConfigFiles(functionGroups, provider, awsAccountId)

	// Use goroutines to deploy functions in parallel, and ensure all finishes
	// Due to CPU and memory constraints, by default, we will deploy 2 serverless.yml files in parallel and wait for them to finish before deploying the next 2
	var wg sync.WaitGroup
	var counter uint64 = 0
	parallelDeployment := 2

	for i := 0; i < len(functionGroups); {
		for parallelIndex := 0; parallelIndex < parallelDeployment; parallelIndex++ {
			if i < len(functionGroups) {
				wg.Add(1)
				go func(functionGroup []*common.Function, index int) {
					defer wg.Done()
					log.Debugf("Deploying serverless-%d.yml", index)
					// Deploy serverless functions and update the function endpoints
					functionToURLMapping := DeployServerless(index)

					if functionToURLMapping == nil {
						CleanAWSLambda(functions)                               // Clean up all deployed functions before exiting
						log.Fatalf("Failed to deploy serverless-%d.yml", index) // Immediately terminate deployment for fast feedback
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

	log.Debugf("Deployed all %d serverless.yml files", len(functionGroups))
}

// CleanAWSLambda cleans up the AWS Lambda deployment environment by deleting all serverless.yml files and the ECR private repository
func CleanAWSLambda(functions []*common.Function) {
	cleanAWSElasticContainerRegistry()

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

	if counter != uint64(len(functionGroups)) {
		log.Errorf("Deleted %d out of %d serverless.yml files", counter, len(functionGroups))
		return
	}

	log.Debugf("Deleted all serverless.yml files")
}

// cleanAWSElasticContainerRegistry cleans up the AWS Elastic Container Registry by deleting the private repository if it exists
func cleanAWSElasticContainerRegistry() {
	// Check if ECR private repository exists, if so, delete it
	checkExistECRRepoCmd := exec.Command("aws", "ecr", "describe-repositories", "--repository-name", common.AwsTraceFuncRepositoryName, "--region", common.AwsRegion)
	err := checkExistECRRepoCmd.Run()
	if err == nil {
		// Delete ECR private repository
		deleteECRRepoCmd := exec.Command("aws", "ecr", "delete-repository", "--repository-name", common.AwsTraceFuncRepositoryName, "--region", common.AwsRegion, "--force")
		err = deleteECRRepoCmd.Run()
		if err != nil {
			log.Errorf("Failed to delete ECR private repository: %s", err)
		}
	}
}

// cleanAWSCloudWatchLogGroups cleans up the AWS CloudWatch log groups by deleting all log groups with the prefix "/aws/lambda/trace-func-"
func cleanAWSCloudWatchLogGroups() {
	// Check if CloudWatch log groups exist, if so, delete them
	logGroupPrefix := fmt.Sprintf("/aws/lambda/%s-", common.FunctionNamePrefix)

	checkExistLogGroupsCmd := exec.Command("aws", "logs", "describe-log-groups", "--log-group-name-prefix", logGroupPrefix, "--query", "logGroups[*].logGroupName", "--output", "json")
	stdOutstdErr, err := checkExistLogGroupsCmd.CombinedOutput()
	if err == nil {
		var logGroupNames []string

		// Extract log group names from JSON output
		// Example JSON output: ["/aws/lambda/trace-func-0", "/aws/lambda/trace-func-1"]
		logGroupNamesRaw := strings.Split(string(stdOutstdErr), "\"")
		for i := 1; i < len(logGroupNamesRaw); i += 2 {
			logGroupNames = append(logGroupNames, logGroupNamesRaw[i])
		}
		log.Debugf("Found %d CloudWatch log groups to delete: %v", len(logGroupNames), logGroupNames)

		for _, logGroupName := range logGroupNames {
			deleteLogGroupCmd := exec.Command("aws", "logs", "delete-log-group", "--log-group-name", logGroupName)
			err = deleteLogGroupCmd.Run()
			if err != nil {
				log.Fatalf("Failed to delete CloudWatch log group %s: %s", logGroupName, err)
			}
		}
	}
}

// initAWSLambda initializes the AWS Lambda deployment environment by checking dependencies, cleaning up previous resources, and initialising ECR repository through initECRRepository
func initAWSLambda(functions []*common.Function, provider string) (string, [][]*common.Function) {
	// Check if all required dependencies are installed
	log.Debug("Checking dependencies for AWS deployment")
	checkDependencies()

	// Clean up previous resources, if any
	log.Debug("Checking and cleaning up previous AWS Lambda resources")
	functionGroups := separateFunctions(functions)
	createSlsConfigFiles(functionGroups, provider, "") // serverless.yml files created do not require AWS account ID
	CleanAWSLambda(functions)
	cleanAWSCloudWatchLogGroups() // Clean up CloudWatch log groups (in rare occasions, log groups persist even after `sls remove`)

	// Create a Private ECR Repository and Upload the Docker Image
	log.Debug("Initialising ECR Repository for AWS Lambda deployment")
	awsAccountId := obtainAWSAccountId()
	initECRRepository(awsAccountId)

	log.Debug("AWS Lambda is ready for deployment")
	return awsAccountId, functionGroups
}

// initECRRepository creates a private ECR repository and uploads the default Docker image to the repository using AWS CLI and Docker CLI, terminating the program if any command fails
func initECRRepository(awsAccountId string) {
	originalDockerImageUri := fmt.Sprintf("ghcr.io/vhive-serverless/%s:latest", common.AwsTraceFuncRepositoryName)
	awsEcrRepositoryFormat := fmt.Sprintf("%s.dkr.ecr.%s.amazonaws.com", awsAccountId, common.AwsRegion)
	uploadedDockerImageUri := fmt.Sprintf("%s/%s:latest", awsEcrRepositoryFormat, common.AwsTraceFuncRepositoryName)

	createRepoCmd := exec.Command("aws", "ecr", "create-repository", "--repository-name", common.AwsTraceFuncRepositoryName, "--region", common.AwsRegion)
	err := createRepoCmd.Run()
	if err != nil {
		log.Fatalf("Failed to create ECR private repository: %s", err)
	}

	dockerLoginECRCmd := exec.Command("sh", "-c", fmt.Sprintf("aws ecr get-login-password --region %s | docker login --username AWS --password-stdin %s", common.AwsRegion, awsEcrRepositoryFormat))
	err = dockerLoginECRCmd.Run()
	if err != nil {
		log.Fatalf("Failed to log Docker into ECR private repository: %s", err)
	}

	pullImageCmd := exec.Command("docker", "pull", originalDockerImageUri)
	err = pullImageCmd.Run()
	if err != nil {
		log.Fatalf("Failed to pull standard image from GHCR: %s", err)
	}

	updateImageTagCmd := exec.Command("docker", "tag", originalDockerImageUri, uploadedDockerImageUri)
	err = updateImageTagCmd.Run()
	if err != nil {
		log.Fatalf("Failed to update image tag: %s", err)
	}

	uploadECRCmd := exec.Command("docker", "push", uploadedDockerImageUri)
	err = uploadECRCmd.Run()
	if err != nil {
		log.Fatalf("Failed to upload image to ECR private repository: %s", err)
	}
}

// checkDependencies checks if all required dependencies (AWS CLI, Docker CLI, Serverless.com framework) are installed, terminating the program if any command fails
func checkDependencies() {
	// Check if AWS CLI is installed
	awsCliCheckCmd := exec.Command("aws", "--version")
	err := awsCliCheckCmd.Run()
	if err != nil {
		log.Fatalf("AWS CLI is not installed: %s", err)
	}

	// Check if Docker is installed
	dockerCheckCmd := exec.Command("docker", "--version")
	err = dockerCheckCmd.Run()
	if err != nil {
		log.Fatalf("Docker is not installed: %s", err)
	}

	// Check if Serverless Framework is installed
	serverlessCheckCmd := exec.Command("sls", "--version")
	err = serverlessCheckCmd.Run()
	if err != nil {
		log.Fatalf("Serverless Framework is not installed: %s", err)
	}
}

// obtainAWSAccountId retrieves the AWS account ID using the AWS CLI, terminating the program if the command fails
func obtainAWSAccountId() string {
	obtainAwdAccountIdCmd := exec.Command("aws", "sts", "get-caller-identity", "--query", "Account", "--output", "text")
	awsAccountIdRaw, err := obtainAwdAccountIdCmd.CombinedOutput()
	if err != nil {
		log.Fatalf("Failed to retrieve AWS account ID: %s", err)
	}
	return strings.TrimSpace(string(awsAccountIdRaw))
}

// separateFunctions splits functions into groups of 60 due to AWS CloudFormation template resource limit (500 resources per template) and IAM maximum policy size (10240 bytes)
func separateFunctions(functions []*common.Function) [][]*common.Function {
	var functionGroups [][]*common.Function
	groupSize := 60

	for i := 0; i < len(functions); i += groupSize {
		end := i + groupSize
		if end > len(functions) {
			end = len(functions)
		}
		functionGroups = append(functionGroups, functions[i:end])
	}

	return functionGroups
}

// createSlsConfigFiles creates serverless.yml files for each group of functions
func createSlsConfigFiles(functionGroups [][]*common.Function, provider string, awsAccountId string) {
	for i := 0; i < len(functionGroups); i++ {
		log.Debugf("Creating serverless-%d.yml", i)
		serverless := awsServerless{}
		serverless.CreateHeader(i, provider)

		for j := 0; j < len(functionGroups[i]); j++ {
			serverless.AddFunctionConfig(functionGroups[i][j], provider, awsAccountId)
		}

		serverless.CreateServerlessConfigFile(i)
	}
}
