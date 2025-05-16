package deployment

import (
	"fmt"
	"os"
	"os/exec"
	"time"

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
	config    *Config
}

func newAzureFunctionsDeployer() *azureFunctionsDeployer {
	return &azureFunctionsDeployer{}
}

func (afd *azureFunctionsDeployer) Deploy(cfg *config.Configuration) {
	afd.functions = cfg.Functions
	afd.config = DeployAzureFunctions(afd.functions)
}

func (afd *azureFunctionsDeployer) Clean() {
	CleanAzureFunctions(afd.config, afd.functions)
}

func DeployAzureFunctions(functions []*common.Function) *Config {
	// 1. Copy exec_func.py to azurefunctions_setup
	// 2. Initialize resources required for Azure Functions deployment
	// 3. Create function folders
	// 4. Zip function folders
	// 5. Deploy the functions to Azure Functions

	// Load azurefunctionsconfig yaml file
	config, err := LoadConfig("azurefunctions_setup/azurefunctionsconfig.yaml")
	if err != nil {
		log.Fatalf("Error loading azure functions config yaml: %s", err)
	}

	// Set unique names for Azure Resources
	timestamp := time.Now().Format("150405")                                                                      // HHMMSS format
	config.AzureConfig.ResourceGroup = fmt.Sprintf("%s-%s", config.AzureConfig.ResourceGroup, timestamp)          // invitro-rg-XXXXXX
	config.AzureConfig.StorageAccountName = fmt.Sprintf("%s%s", config.AzureConfig.StorageAccountName, timestamp) // invitrostorageXXXXXX
	config.AzureConfig.FunctionAppName = fmt.Sprintf("%s-%s", config.AzureConfig.FunctionAppName, timestamp)      // invitro-functionapp-XXXXXX

	// Define the base directory containing functions to be zipped individually
	baseDir := "azure_functions_for_zip"
	sharedWorkloadDir := filepath.Join("azurefunctions_setup", "shared_azure_workload")
	zipBaseDir := "."

	// 1. Run script to copy workload
	if err := CopyPythonWorkload("server/trace-func-py/exec_func.py", "azurefunctions_setup/shared_azure_workload/exec_func.py"); err != nil {
		log.Fatalf("Error copying Python workload: %s", err)
	}

	// 2. Initialize resources required for Azure Functions deployment
	InitAzureFunctions(config, functions)

	// 3. Create function folders
	if err := CreateFunctionFolders(baseDir, sharedWorkloadDir, functions); err != nil {
		log.Fatalf("Error setting up function folders required for zipping: %s", err)
	}

	// 4. Zip function folders
	if err := ZipFunctionAppFiles(baseDir, functions); err != nil {
		log.Fatalf("Error zipping function app files for deployment: %s", err)
	}

	// 5. Deploy the function to Azure Functions
	if err := DeployFunctions(config, zipBaseDir, functions); err != nil {
		log.Fatalf("Error deploying function: %s", err)
	}

	return config
}

func CleanAzureFunctions(config *Config, functions []*common.Function) {
	log.Infof("Performing cleanup of experiment...")

	baseDir := "azure_functions_for_zip"

	// Call the cleanup function to delete temp folders and files
	if err := CleanUpDeploymentFiles(baseDir, functions); err != nil {
		log.Errorf("Error during cleanup of local files: %s", err)
	} else {
		log.Debug("Cleanup of temp folders zip files completed successfully.")
	}

	// Delete Azure resources
	if err := DeleteResourceGroup(config); err != nil {
		log.Errorf("Error during Azure resource cleanup: %v", err)
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

func InitAzureFunctions(config *Config, functions []*common.Function) {
	// 1. Create Resource Group
	// 2. Create Storage Account
	// 3. Create Function Apps + Set Settings For Each App

	// 1. Create Resource Group
	if err := CreateResourceGroup(config); err != nil {
		log.Fatalf("Error during Resource Group creation: %s", err)
	}

	// 2. Create Storage Account
	if err := CreateStorageAccount(config); err != nil {

		cleanupErr := DeleteResourceGroup(config)
		if cleanupErr != nil {
			log.Errorf("Failed to delete resource group during cleanup: %v", cleanupErr)
		}

		log.Fatalf("Error during Storage Account creation: %s", err)
	}

	// 3. Create Function Apps + Set Settings For Each App
	for i := 0; i < len(functions); i++ {
		functionAppName := fmt.Sprintf("%s-%d", config.AzureConfig.FunctionAppName, i)

		if err := CreateFunctionApp(config, functionAppName); err != nil {

			cleanupErr := DeleteResourceGroup(config)
			if cleanupErr != nil {
				log.Errorf("Failed to delete resource group during cleanup: %v", cleanupErr)
			}

			log.Fatalf("Error during Function App creation: %s", err)
		}

		// Set SCM_DO_BUILD_DURING_DEPLOYMENT
		if err := SetSCMSettings(config, functionAppName); err != nil {

			cleanupErr := DeleteResourceGroup(config)
			if cleanupErr != nil {
				log.Errorf("Failed to delete resource group during cleanup: %v", cleanupErr)
			}

			log.Fatalf("failed to set SCM settings: %s", err)
		}

		// Set ENABLE_ORYX_BUILD
		if err := SetORYXSettings(config, functionAppName); err != nil {

			cleanupErr := DeleteResourceGroup(config)
			if cleanupErr != nil {
				log.Errorf("Failed to delete resource group during cleanup: %v", cleanupErr)
			}

			log.Fatalf("failed to set Oryx settings: %s", err)
		}
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
func CreateFunctionApp(config *Config, functionAppName string) error {
	cmd := exec.Command("az", "functionapp", "create",
		"--name", functionAppName,
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

	log.Infof("Function app %s created successfully.", functionAppName)
	return nil
}

// SetSCMSettings configures remote build settings for the Azure Function App
func SetSCMSettings(config *Config, functionAppName string) error {
	cmd := exec.Command("az", "functionapp", "config", "appsettings", "set",
		"--name", functionAppName,
		"--resource-group", config.AzureConfig.ResourceGroup,
		"--settings", "SCM_DO_BUILD_DURING_DEPLOYMENT=true")

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to set SCM_DO_BUILD_DURING_DEPLOYMENT: %w", err)
	}

	log.Debugf("SCM_DO_BUILD_DURING_DEPLOYMENT setting configured successfully.")
	return nil
}

// SetORYXSettings configures remote build settings for the Azure Function App
func SetORYXSettings(config *Config, functionAppName string) error {
	cmd := exec.Command("az", "functionapp", "config", "appsettings", "set",
		"--name", functionAppName,
		"--resource-group", config.AzureConfig.ResourceGroup,
		"--settings", "ENABLE_ORYX_BUILD=true")

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to set ENABLE_ORYX_BUILD: %w", err)
	}

	log.Debugf("ENABLE_ORYX_BUILD setting configured successfully.")
	return nil
}

/* Function to create folders and copy files to the folders */

func CreateFunctionFolders(baseDir, sharedWorkloadDir string, functions []*common.Function) error {
	for i := 0; i < len(functions); i++ {
		folderName := fmt.Sprintf("function%d", i)
		folderPath := filepath.Join(baseDir, folderName)

		// Create the function folder
		if err := os.MkdirAll(folderPath, os.ModePerm); err != nil {
			return fmt.Errorf("failed to create folder %s: %w", folderPath, err)
		}

		// Build full paths to shared workload files
		workloadPy := filepath.Join(sharedWorkloadDir, "azurefunctionsworkload.py")
		execFuncPy := filepath.Join(sharedWorkloadDir, "exec_func.py")
		functionJSON := filepath.Join(sharedWorkloadDir, "function.json")

		// Copy files into the function folder
		if err := common.CopyFile(workloadPy, filepath.Join(folderPath, "azurefunctionsworkload.py")); err != nil {
			return fmt.Errorf("failed to copy azurefunctionsworkload.py to %s: %w", folderPath, err)
		}
		if err := common.CopyFile(execFuncPy, filepath.Join(folderPath, "exec_func.py")); err != nil {
			return fmt.Errorf("failed to copy exec_func.py to %s: %w", folderPath, err)
		}
		if err := common.CopyFile(functionJSON, filepath.Join(folderPath, "function.json")); err != nil {
			return fmt.Errorf("failed to copy function.json to %s: %w", folderPath, err)
		}
	}

	log.Debugf("Created %d function folders under %s", len(functions), baseDir)
	return nil
}

/* Functions for zipping created function folders */

func ZipFunctionAppFiles(baseDir string, functions []*common.Function) error {
	for i := 0; i < len(functions); i++ {
		folderName := fmt.Sprintf("function%d", i)
		folderPath := filepath.Join(baseDir, folderName)
		zipFileName := fmt.Sprintf("function%d.zip", i)

		// Use bash to zip the contents of azure_functions_for_zip/ along with host.json and requirements.txt
		cmd := exec.Command("bash", "-c",
			fmt.Sprintf("cd %s && zip -r ../%s %s && cd .. && zip -j %s azurefunctions_setup/host.json azurefunctions_setup/requirements.txt", baseDir, zipFileName, folderName, zipFileName))

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to zip function folder %s into %s: %w", folderPath, zipFileName, err)
		}
		log.Debugf("Created zip file %s for function folder %s", zipFileName, folderName)

	}
	log.Infof("Successfully zipped %d functions.", len(functions))
	return nil
}

/* Function for deploying zipped functions */

func DeployFunctions(config *Config, baseDir string, functions []*common.Function) error {
	log.Infof("Deploying %d functions to Azure Function Apps...", len(functions))

	for i := 0; i < len(functions); i++ {
		functionAppName := fmt.Sprintf("%s-%d", config.AzureConfig.FunctionAppName, i)
		zipFileName := fmt.Sprintf("function%d.zip", i)
		zipFilePath := filepath.Join(baseDir, zipFileName)

		// Deploy the zip file to Azure Function App using CLI
		cmd := exec.Command("az", "functionapp", "deployment", "source", "config-zip",
			"--name", functionAppName,
			"--resource-group", config.AzureConfig.ResourceGroup,
			"--src", zipFilePath,
			"--build-remote", "true")

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to deploy %s to function app %s: %w", zipFilePath, functionAppName, err)
		}

		// Storing endpoint for each function
		functions[i].Endpoint = fmt.Sprintf("https://%s.azurewebsites.net/api/function%d", functionAppName, i)
		log.Infof("Function %s set to %s", functions[i].Name, functions[i].Endpoint)

	}
	log.Infof("Successfully deployed all %d functions.", len(functions))
	return nil
}

/* Functions for clean up */

// Clean up temporary files and folders after deployment
func CleanUpDeploymentFiles(baseDir string, functions []*common.Function) error {

	// Remove the base directory containing function folders
	if err := os.RemoveAll(baseDir); err != nil {
		return fmt.Errorf("failed to remove directory %s: %w", baseDir, err)
	}
	log.Debugf("Successfully removed directory: %s", baseDir)

	// Remove each individual function zip file
	for i := 0; i < len(functions); i++ {
		zipFileName := fmt.Sprintf("function%d.zip", i)
		if err := os.Remove(zipFileName); err != nil {
			return fmt.Errorf("failed to remove zip file %s: %w", zipFileName, err)
		}
		log.Debugf("Successfully removed zip file: %s", zipFileName)
	}

	// Remove the copied exec_func.py from shared workload
	execFuncPath := "azurefunctions_setup/shared_azure_workload/exec_func.py"
	if err := os.Remove(execFuncPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove copied exec_func.py: %w", err)
	}
	log.Debugf("Successfully removed copied exec_func.py from shared workload.")
	return nil
}

// DeleteResourceGroup deletes the Azure Resource Group
func DeleteResourceGroup(config *Config) error {

	// Construct the Azure CLI command to delete the resource group
	dltResourceGrpCmd := exec.Command("az", "group", "delete",
		"--name", config.AzureConfig.ResourceGroup, // Resource group name
		"--yes", // Skip confirmation prompt
		"--no-wait")

	// Execute the command
	if err := dltResourceGrpCmd.Run(); err != nil {
		return fmt.Errorf("failed to delete resource group: %w", err)
	}

	log.Debugf("Resource group %s deleted successfully.", config.AzureConfig.ResourceGroup)
	return nil
}
