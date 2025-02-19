package deployment

import (
	"fmt"
	"os"
	"os/exec"

	"path/filepath"

	log "github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/pkg/common"
	"github.com/vhive-serverless/loader/pkg/config"
	"gopkg.in/yaml.v3"
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

// CommandRunner allows us to mock exec.Command for testing
type CommandRunner func(name string, arg ...string) *exec.Cmd

func newAzureFunctionsDeployer() *azureFunctionsDeployer {
	return &azureFunctionsDeployer{}
}

func (afd *azureFunctionsDeployer) Deploy(cfg *config.Configuration) {
	afd.functions = cfg.Functions
	DeployAzureFunctions(afd.functions)
}

func (afd *azureFunctionsDeployer) Clean() {
	CleanAzureFunctions()
}

func DeployAzureFunctions(functions []*common.Function) {
	// 1. Copy exec_func.py to azurefunctions_setup
	// 2. Initialize resources required for Azure Functions deployment
	// 3. Create function folders
	// 4. Zip function folders
	// 5. Deploy the function to Azure Functions

	// Load azurefunctionsconfig yaml file
	config, err := LoadConfig("azurefunctions_setup/azurefunctionsconfig.yaml")
	if err != nil {
		log.Fatalf("Error loading azure functions config yaml: %s", err)
	}

	baseDir := "azure_functions_for_zip"

	// 1. Run script to copy workload
	if err := CopyPythonWorkload("server/trace-func-py/exec_func.py", "azurefunctions_setup/shared_azure_workload/exec_func.py"); err != nil {
		log.Fatalf("Error copying Python workload: %s", err)
	}

	// 2. Initialize resources required for Azure Functions deployment
	initAzureFunctions(config)

	// 3. Create function folders
	if err := CreateFunctionFolders(baseDir, functions); err != nil {
		log.Fatalf("Error setting up function folders required for zipping: %s", err)
	}

	// 4. Zip function folders
	if err := ZipFunctionAppFiles(); err != nil {
		log.Fatalf("Error zipping function app files for deployment: %s", err)
	}

	// 5. Deploy the function to Azure Functions
	if err := DeployFunction(config, functions, exec.Command); err != nil {
		log.Fatalf("Error deploying function: %s", err)
	}
}

func CleanAzureFunctions() {
	// Load azurefunctionsconfig yaml file
	config, err := LoadConfig("azurefunctions_setup/azurefunctionsconfig.yaml")
	if err != nil {
		log.Fatalf("Error loading azure functions config yaml: %s", err)
	}

	log.Infof("Performing cleanup of experiment...")

	// Call the cleanup function to delete temp folders and files
	if err := CleanUpDeploymentFiles("azure_functions_for_zip", "azurefunctions.zip"); err != nil {
		log.Errorf("Error during cleanup: %s", err)
	} else {
		log.Debug("Cleanup of temp folders zip files completed successfully.")
	}

	// Delete Azure resources
	if err := DeleteResourceGroup(config); err != nil {
		log.Errorf("Cleanup failed: %v", err)
	} else {
		log.Infof("Cleanup completed successfully.")
	}
}

/* Copy the exec_func.py file to the destination using the CopyFile function from utilities */
func CopyPythonWorkload(srcPath, dstPath string) error {
	log.Infof("Copying workload...")
	if err := common.CopyFile(srcPath, dstPath); err != nil {
		return fmt.Errorf("failed to copy exec_func.py to %s: %w", dstPath, err)
	}
	log.Infof("Workload copied successfully.")
	return nil
}

/* Functions for initializing resources required for Azure Functions deployment */

func initAzureFunctions(config *Config) {
	// 1. Create Resource Group
	// 2. Create Storage Account
	// 3. Create Function App
	// 4. Set SCM_DO_BUILD_DURING_DEPLOYMENT
	// 5. Set ENABLE_ORYX_BUILD

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

	// 4. Set SCM_DO_BUILD_DURING_DEPLOYMENT
	if err := SetSCMSettings(config); err != nil {
		log.Fatalf("failed to set SCM settings: %s", err)
	}

	// 5. Set ENABLE_ORYX_BUILD
	if err := SetORYXSettings(config); err != nil {
		log.Fatalf("failed to set Oryx settings: %s", err)
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

	log.Debugf("Resource group %s created successfully.", config.AzureConfig.ResourceGroup)
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

	log.Debugf("Storage account %s created successfully.", config.AzureConfig.StorageAccountName)
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

	log.Debugf("Function app %s created successfully.", config.AzureConfig.FunctionAppName)
	return nil
}

// SetSCMSettings configures remote build settings for the Azure Function App
func SetSCMSettings(config *Config) error {
	cmd := exec.Command("az", "functionapp", "config", "appsettings", "set",
		"--name", config.AzureConfig.FunctionAppName,
		"--resource-group", config.AzureConfig.ResourceGroup,
		"--settings", "SCM_DO_BUILD_DURING_DEPLOYMENT=true")

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to set SCM_DO_BUILD_DURING_DEPLOYMENT: %w", err)
	}

	log.Debugf("SCM_DO_BUILD_DURING_DEPLOYMENT setting configured successfully.")
	return nil
}

// SetORYXSettings configures remote build settings for the Azure Function App
func SetORYXSettings(config *Config) error {
	cmd := exec.Command("az", "functionapp", "config", "appsettings", "set",
		"--name", config.AzureConfig.FunctionAppName,
		"--resource-group", config.AzureConfig.ResourceGroup,
		"--settings", "ENABLE_ORYX_BUILD=true")

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to set ENABLE_ORYX_BUILD: %w", err)
	}

	log.Debugf("ENABLE_ORYX_BUILD setting configured successfully.")
	return nil
}

/* Functions for creating function folders before zipping */

// Function to create folders and copy files to the folders
func CreateFunctionFolders(baseDir string, function []*common.Function) error {
	for i := 0; i < len(function); i++ {
		folderName := fmt.Sprintf("function%d", i)
		folderPath := filepath.Join(baseDir, folderName)

		// Create the function folder
		if err := os.MkdirAll(folderPath, os.ModePerm); err != nil {
			return fmt.Errorf("failed to create folder %s: %w", folderPath, err)
		}

		// Copy azurefunctionsworkload.py, exec_func.py and function.json into each function folder
		if err := common.CopyFile("azurefunctions_setup/shared_azure_workload/azurefunctionsworkload.py", filepath.Join(folderPath, "azurefunctionsworkload.py")); err != nil {
			return fmt.Errorf("failed to copy azureworkload.py to %s: %w", folderPath, err)
		}
		if err := common.CopyFile("azurefunctions_setup/shared_azure_workload/exec_func.py", filepath.Join(folderPath, "exec_func.py")); err != nil {
			return fmt.Errorf("failed to copy exec_func.py to %s: %w", folderPath, err)
		}
		if err := common.CopyFile("azurefunctions_setup/shared_azure_workload/function.json", filepath.Join(folderPath, "function.json")); err != nil {
			return fmt.Errorf("failed to copy function.json to %s: %w", folderPath, err)
		}
	}
	log.Debugf("Created %d function folders with copies of azureworkload.py, exec_func.py and function.json under %s folder.\n", len(function), baseDir)
	return nil
}

/* Functions for zipping created function folders */

func ZipFunctionAppFiles() error {
	// Use bash to zip the contents of azure_functions_for_zip/ along with host.json and requirements.txt
	cmd := exec.Command("bash", "-c", "cd azure_functions_for_zip && zip -r ../azurefunctions.zip . && cd .. && zip -j azurefunctions.zip azurefunctions_setup/host.json azurefunctions_setup/requirements.txt")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to zip function app files for deployment: %w", err)
	}

	log.Debug("Functions for deployment zipped successfully.")
	return nil
}

/* Functions for deploying zipped functions */

func DeployFunction(config *Config, function []*common.Function, runner CommandRunner) error {
	log.Infof("Deploying %d functions to Azure Function App...", len(function))

	// Path to the zip file that contains the Python binary and other resources for deployment to Azure
	zipFilePath := "azurefunctions.zip"

	// Deploy the zip file to Azure Function App using CLI
	cmd := runner("az", "functionapp", "deployment", "source", "config-zip",
		"--name", config.AzureConfig.FunctionAppName,
		"--resource-group", config.AzureConfig.ResourceGroup,
		"--src", zipFilePath,
		"--build-remote", "true")

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to deploy zip file to function app: %w", err)
	}

	log.Infof("Deployed all %d functions successfully, with the following endpoints.", len(function))

	// Storing endpoint for each function
	for i := 0; i < len(function); i++ {
		function[i].Endpoint = fmt.Sprintf("https://%s.azurewebsites.net/api/function%d", config.AzureConfig.FunctionAppName, i)
		log.Infof("Function %s set to %s", function[i].Name, function[i].Endpoint)
	}

	return nil
}

/* Functions for clean up */

// Clean up temporary files and folders after deployment
func CleanUpDeploymentFiles(baseDir string, zipFile string) error {

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

// DeleteResourceGroup deletes the Azure Resource Group
func DeleteResourceGroup(config *Config) error {

	// Construct the Azure CLI command to delete the resource group
	dltResourceGrpCmd := exec.Command("az", "group", "delete",
		"--name", config.AzureConfig.ResourceGroup, // Resource group name
		"--yes") // Skip confirmation prompt

	// Execute the command
	if err := dltResourceGrpCmd.Run(); err != nil {
		return fmt.Errorf("failed to delete resource group: %w", err)
	}

	log.Debugf("Resource group %s deleted successfully.", config.AzureConfig.ResourceGroup)
	return nil
}
