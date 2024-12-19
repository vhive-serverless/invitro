package deployment

import (
	"fmt"
	"io"
	"os"
	"os/exec"

	"path/filepath"

	log "github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/pkg/common"
	"github.com/vhive-serverless/loader/pkg/config"
	"gopkg.in/yaml.v2"
)

// Config struct to hold Azure Function deployment configuration
type Config struct {
	AzureConfig struct {
		ResourceGroup      string `yaml:"resource_group"`
		StorageAccountName string `yaml:"storage_account_name"`
		FunctionAppName    string `yaml:"function_app_name"`
		Location           string `yaml:"location"`
	} `yaml:"azurefunctionsconfig"`
}

type azureFunctionsDeployer struct {
	functions []*common.Function
}

func newAzureFunctionsDeployer() *azureFunctionsDeployer {
	return &azureFunctionsDeployer{}
}

func (afd *azureFunctionsDeployer) Deploy(cfg *config.Configuration) {
	afd.functions = cfg.Functions
	deployAzureFunctions(afd.functions)
}

func (afd *azureFunctionsDeployer) Clean() {
	cleanAzureFunctions(afd.functions)
}

func deployAzureFunctions(functions []*common.Function) {

	// 1. Initialize resources required for Azure Functions deployment
	// 2. Create function folders
	// 3. Zip function folders
	// 4. Deploy the function to Azure Functions

	// Load azurefunctionsconfig yaml file
	config, err := LoadConfig("azurefunctions_setup/azurefunctionsconfig.yaml")
	if err != nil {
		log.Fatalf("Error loading azure functions config yaml: %s", err)
	}

	baseDir := "azure_functions_for_zip"

	// 1. Initialize resources required for Azure Functions deployment
	initAzureFunctions(config)

	// 2. Create function folders
	if err := createFunctionFolders(baseDir, functions); err != nil {
		log.Fatalf("Error setting up function folders required for zipping: %s", err)
	}

	// 3. Zip function folders
	if err := ZipFunctionAppFiles(); err != nil {
		log.Fatalf("Error zipping function app files for deployment: %s", err)
	}

	// 4. Deploy the function to Azure Functions
	if err := DeployFunction(config, functions); err != nil {
		log.Fatalf("Error deploying function: %s", err)
	}

}

func cleanAzureFunctions(functions []*common.Function) {

	//Delete created folders
	//Delete zip file
	//Delete Azure resources

}

/* Functions for initializing resources required for Azure Functions deployment */

func initAzureFunctions(config *Config) {

	// 1.Create Resource Group
	// 2.Create Storage Account
	// 3.Create Function App
	// 4.Set WEBSITE_RUN_FROM_PACKAGE

	//checkDependencies()	ToDo: Check if all required dependencies are installed on VM

	// 1. Create Resource Group
	if err := CreateResourceGroup(config); err != nil {
		log.Fatalf("Error during Resource Group creation: %s", err)
	}

	// 2. Create Storage Account
	if err := CreateStorageAccount(config); err != nil {
		log.Fatalf("Error during Storage Account creation: %s", err)
	}

	// 3. Create Function App
	if err := CreateFunctionApp(config); err != nil {
		log.Fatalf("Error during Function App creation: %s", err)
	}

	// 4. Set WEBSITE_RUN_FROM_PACKAGE
	if err := SetWebsiteRunFromPackage(config); err != nil {
		log.Fatalf("Error setting WEBSITE_RUN_FROM_PACKAGE: %s", err)
	}

	log.Info("Azure Functions environment for deployment initialized successfully.")
}

// LoadConfig reads the YAML configuration file
func LoadConfig(filePath string) (*Config, error) {
	config := &Config{}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	err = yaml.Unmarshal(data, config)
	return config, err
}

// CreateResourceGroup creates an Azure Resource Group: {az group create --name <resource-group> --location <location>}
func CreateResourceGroup(config *Config) error {
	createResourceGroupCmd := exec.Command("az", "group", "create",
		"--name", config.AzureConfig.ResourceGroup,
		"--location", config.AzureConfig.Location)

	if err := createResourceGroupCmd.Run(); err != nil {
		return fmt.Errorf("failed to create resource group: %w", err)
	}

	log.Infof("Resource group %s created successfully.", config.AzureConfig.ResourceGroup)
	return nil
}

// CreateStorageAccount creates an Azure Storage Account : {az storage account create --name <storage-account-name> --resource-group <resource-group> --location <location> --sku Standard_LRS}
func CreateStorageAccount(config *Config) error {
	cmd := exec.Command("az", "storage", "account", "create",
		"--name", config.AzureConfig.StorageAccountName,
		"--resource-group", config.AzureConfig.ResourceGroup,
		"--location", config.AzureConfig.Location,
		"--sku", "Standard_LRS")

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create storage account: %w", err)
	}

	log.Infof("Storage account %s created successfully.", config.AzureConfig.StorageAccountName)
	return nil
}

// CreateFunctionApp creates an Azure Function App: {az functionapp create --name <function-app-name> --resource-group <resource-group> --storage-account <storage-account-name> --consumption-plan-location <location> --runtime python --runtime-version 3.10 --os-type linux --functions-version 4}
func CreateFunctionApp(config *Config) error {
	cmd := exec.Command("az", "functionapp", "create",
		"--name", config.AzureConfig.FunctionAppName,
		"--resource-group", config.AzureConfig.ResourceGroup,
		"--storage-account", config.AzureConfig.StorageAccountName,
		"--consumption-plan-location", config.AzureConfig.Location,
		"--runtime", "python",
		"--runtime-version", "3.10",
		"--os-type", "linux",
		"--functions-version", "4")

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create function app: %w", err)
	}

	log.Infof("Function app %s created successfully.", config.AzureConfig.FunctionAppName)
	return nil
}

// SetWebsiteRunFromPackage configures the function app to run from a zip package
func SetWebsiteRunFromPackage(config *Config) error {
	cmd := exec.Command("az", "functionapp", "config", "appsettings", "set",
		"--name", config.AzureConfig.FunctionAppName,
		"--resource-group", config.AzureConfig.ResourceGroup,
		"--settings", "WEBSITE_RUN_FROM_PACKAGE=1")

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to set WEBSITE_RUN_FROM_PACKAGE: %w", err)
	}

	log.Info("WEBSITE_RUN_FROM_PACKAGE set successfully.")
	return nil
}

/* Functions for creating function folders before zipping */

// Function to create folders and copy files to the folders
func createFunctionFolders(baseDir string, function []*common.Function) error {

	for i := 0; i < len(function); i++ {
		folderName := fmt.Sprintf("function%d", i)
		folderPath := filepath.Join(baseDir, folderName)

		// Create the function folder
		if err := os.MkdirAll(folderPath, os.ModePerm); err != nil {
			return fmt.Errorf("failed to create folder %s: %w", folderPath, err)
		}

		// Copy azureworkload.py, requirements.txt, and function.json into each function folder
		if err := copyFile("azurefunctions_setup/shared_azure_workload/azureworkload.py", filepath.Join(folderPath, "azureworkload.py")); err != nil {
			return fmt.Errorf("failed to copy azureworkload.py to %s: %w", folderPath, err)
		}
		if err := copyFile("azurefunctions_setup/shared_azure_workload/requirements.txt", filepath.Join(folderPath, "requirements.txt")); err != nil {
			return fmt.Errorf("failed to copy requirements.txt to %s: %w", folderPath, err)
		}
		if err := copyFile("azurefunctions_setup/shared_azure_workload/function.json", filepath.Join(folderPath, "function.json")); err != nil {
			return fmt.Errorf("failed to copy function.json to %s: %w", folderPath, err)
		}
	}

	log.Debugf("Created %d function folders with copies of azureworkload.py, requirements.txt, and function.json under %s folder.\n", len(function), baseDir)
	return nil
}

// Helper function to copy files
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return err
	}

	return destFile.Sync()
}

/* Functions for zipping created function folders */

func ZipFunctionAppFiles() error {

	// Use bash to zip the contents of azure_functions_for_zip/* along with host.json directly into azurefunctions.zip
	cmd := exec.Command("bash", "-c", "cd azure_functions_for_zip && zip -r ../azurefunctions.zip . && cd .. && zip -j azurefunctions.zip azurefunctions_setup/host.json")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to zip function app files for deployment: %w", err)
	}
	log.Info("Functions for deployment zipped successfully.")
	return nil
}

/* Functions for deploying zipped functions */

func DeployFunction(config *Config, function []*common.Function) error {

	log.Infof("Deploying %d functions to Azure Function App...", len(function))

	// Path to the zip file that contains the Python binary and other resources for deployment to Azure
	zipFilePath := "azurefunctions.zip"

	// Deploy the zip file to Azure Function App using CLI
	cmd := exec.Command("az", "functionapp", "deployment", "source", "config-zip",
		"--name", config.AzureConfig.FunctionAppName,
		"--resource-group", config.AzureConfig.ResourceGroup,
		"--src", zipFilePath)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to deploy zip file to function app: %w", err)
	}

	log.Infof("Deployed all %d functions successfully.", len(function))

	// Storing endpoint for each function
	for i := 0; i < len(function); i++ {
		function[i].Endpoint = fmt.Sprintf("https://%s.azurewebsites.net/api/function%d", config.AzureConfig.FunctionAppName, i)
		log.Infof("Function %s set to %s", function[i].Name, function[i].Endpoint)
	}

	// Call the cleanup function after deployment, to delete temp folders and files
	if err := cleanUpDeploymentFiles("azure_functions_for_zip", "azurefunctions.zip"); err != nil {
		log.Errorf("Error during cleanup: %s", err)
	} else {
		log.Info("Deployment and cleanup of zip files completed successfully.")
	}

	//Stop the program after deployment for testing purposes, to remove after invocation is implemented
	log.Info("Stopping program after deployment phase for PR.")
	os.Exit(0) // Exit with status code 0 (successful execution)

	return nil

}

/* Functions for clean up */

// Clean up temporary files and folders after deployment
func cleanUpDeploymentFiles(baseDir string, zipFile string) error {
	// Remove the base directory containing function folders
	if err := os.RemoveAll(baseDir); err != nil {
		return fmt.Errorf("failed to remove directory %s: %w", baseDir, err)
	}
	log.Debugf("Successfully removed directory: %s", baseDir)

	// Remove the zip file used for deployment
	if err := os.Remove(zipFile); err != nil {
		return fmt.Errorf("failed to remove zip file %s: %w", zipFile, err)
	}
	log.Debugf("Successfully removed zip file: %s", zipFile)

	return nil
}
