package deployment_test

import (
	"archive/zip"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vhive-serverless/loader/pkg/common"
	"github.com/vhive-serverless/loader/pkg/driver/deployment"
)

/* --- TEST CASES --- */

// Tests copying of exec_func.py file and verifies the contents.
func TestCopyPythonWorkload(t *testing.T) {

	// Get current working directory
	cwd, err := os.Getwd()
	assert.NoError(t, err)

	// Construct root directory path
	root := filepath.Join(cwd, "..", "..", "..")

	// Set source and destination paths for exec_func.py
	srcPath := filepath.Join(root, "server", "trace-func-py", "exec_func.py")
	dstPath := filepath.Join(root, "azurefunctions_setup", "shared_azure_workload", "exec_func.py")

	// Read content of source file
	srcContent, err := os.ReadFile(srcPath)
	assert.NoError(t, err)

	// Copy the file to destination
	err = deployment.CopyPythonWorkload(srcPath, dstPath)
	assert.NoError(t, err)

	// Ensure destination file is cleaned up automatically
	t.Cleanup(func() {
		_ = os.Remove(dstPath)
	})

	// Read content of destination file
	dstContent, err := os.ReadFile(dstPath)
	assert.NoError(t, err)

	// Verify the contents are the same
	assert.Equal(t, string(srcContent), string(dstContent))
}

// Tests zip file contains the correct files and directory structure.
func TestZipHealth(t *testing.T) {

	// Get current working directory
	cwd, err := os.Getwd()
	assert.NoError(t, err)

	// Construct root directory path
	root := filepath.Join(cwd, "..", "..", "..")

	// Set source and destination paths for exec_func.py
	srcPath := filepath.Join(root, "server", "trace-func-py", "exec_func.py")
	dstPath := filepath.Join(root, "azurefunctions_setup", "shared_azure_workload", "exec_func.py")

	baseDir := filepath.Join(root, "azure_functions_for_zip")
	sharedWorkloadDir := filepath.Join(root, "azurefunctions_setup", "shared_azure_workload")
	expectedFunctionCount := 2

	expectedFunctionFiles := []string{
		"azurefunctionsworkload.py",
		"exec_func.py",
		"function.json",
	}

	expectedRootFiles := []string{
		"requirements.txt",
		"host.json",
	}

	// Create test functions
	functions := []*common.Function{
		{Name: "function0"},
		{Name: "function1"},
	}

	defer cleanupTestArtifacts(t, baseDir, functions)

	// Copy the file to destination
	err = deployment.CopyPythonWorkload(srcPath, dstPath)
	assert.NoError(t, err)

	// Create function folders
	err = deployment.CreateFunctionFolders(baseDir, sharedWorkloadDir, functions)
	assert.NoError(t, err, "Failed to create function folders")

	// Zip the function app files (each function separately)
	err = deployment.ZipFunctionAppFiles(baseDir, functions)
	assert.NoError(t, err, "Failed to create function app zips")

	// Validate each zip file
	for i := 0; i < expectedFunctionCount; i++ {
		zipFilePath := filepath.Join(root, fmt.Sprintf("function%d.zip", i))

		// Check if the zip file exists
		if _, err := os.Stat(zipFilePath); os.IsNotExist(err) {
			t.Fatalf("Zip file does not exist: %s", zipFilePath)
		}

		// Open zip file
		r, err := zip.OpenReader(zipFilePath)
		assert.NoError(t, err, "Failed to open zip file")
		defer r.Close()

		// Prepare expected files map
		expectedFiles := make(map[string]bool)
		functionFolder := fmt.Sprintf("function%d/", i)

		for _, file := range expectedFunctionFiles {
			expectedFiles[functionFolder+file] = false
		}
		for _, file := range expectedRootFiles {
			expectedFiles[file] = false
		}

		// Check the files inside the zip
		for _, f := range r.File {
			filePath := f.Name
			filePath = strings.TrimPrefix(filePath, "./") // Normalize path

			if _, exists := expectedFiles[filePath]; exists {
				expectedFiles[filePath] = true
			}
		}

		// Ensure all expected files are present
		for file, found := range expectedFiles {
			assert.True(t, found, "Missing expected file in zip: "+file)
		}

	}

	t.Log("Zip file structure validation passed!")
}

// Tests loading of config file in /azurefunctions_setup.
func TestLoadConfig(t *testing.T) {

	// Get current working directory
	cwd, err := os.Getwd()
	assert.NoError(t, err)

	// Construct root directory path
	root := filepath.Join(cwd, "..", "..", "..")

	// Set path to config file
	configPath := filepath.Join(root, "azurefunctions_setup", "azurefunctionsconfig.yaml")

	t.Logf("Looking for config at: %s", configPath)

	config, err := deployment.LoadConfig(configPath)
	require.NoError(t, err, "Failed to load config")

	assert.NotEmpty(t, config.AzureConfig.ResourceGroup)
	assert.NotEmpty(t, config.AzureConfig.StorageAccountName)
	assert.NotEmpty(t, config.AzureConfig.FunctionAppName)
	assert.NotEmpty(t, config.AzureConfig.Location)

	t.Log("Config loaded and validated successfully!")
}

// Tests Azure infrastructure creation (Resource Group, Storage Account, Function App).
func TestAzureInfra(t *testing.T) {

	config, functionAppName := setupConfig(t)
	defer cleanupAzureResources(t, config)

	err := deployment.CreateResourceGroup(config)
	require.NoError(t, err, "Failed to create Resource Group")

	err = deployment.CreateStorageAccount(config)
	require.NoError(t, err, "Failed to create Storage Account")

	err = deployment.CreateFunctionApp(config, functionAppName)
	require.NoError(t, err, "Failed to create Function App")

	t.Logf("Function App %s created successfully!", functionAppName)
}

// Tests deployment of zipped function to Azure Function App.
func TestDeployFunction(t *testing.T) {

	// Get current working directory
	cwd, err := os.Getwd()
	assert.NoError(t, err)

	// Construct root directory path
	root := filepath.Join(cwd, "..", "..", "..")

	// Setup Azure login and config
	config, functionAppName := setupConfig(t)

	// Prepare local zipped workload
	functions := prepareZipFile(t)

	// Cleanup after test
	defer cleanupTestArtifacts(t, "azure_functions_for_zip_test", functions)
	defer cleanupAzureResources(t, config)

	err = deployment.CreateResourceGroup(config)
	require.NoError(t, err, "Failed to create Resource Group")

	err = deployment.CreateStorageAccount(config)
	require.NoError(t, err, "Failed to create Storage Account")

	err = deployment.CreateFunctionApp(config, functionAppName)
	require.NoError(t, err, "Failed to create Function App")

	// Set SCM and ORYX settings for proper Azure deployment
	err = deployment.SetSCMSettings(config, functionAppName)
	require.NoError(t, err, "Failed to set SCM_DO_BUILD_DURING_DEPLOYMENT")

	err = deployment.SetORYXSettings(config, functionAppName)
	require.NoError(t, err, "Failed to set ENABLE_ORYX_BUILD")

	// Deploy zip file to Function App
	err = deployment.DeployFunctions(config, root, functions)
	require.NoError(t, err, "Failed to deploy function")

	t.Log("Function deployment successful!")

	// Check if the endpoint is correctly set
	expectedEndpoint := fmt.Sprintf("https://%s.azurewebsites.net/api/function%d", functionAppName, 0)
	assert.Equal(t, expectedEndpoint, functions[0].Endpoint, "Function endpoint does not match expected")
}

/* --- HELPER FUNCTIONS --- */

// Azure login
func setupAzureLogin(t *testing.T) {

	appID := os.Getenv("AZURE_APP_ID")
	password := os.Getenv("AZURE_PASSWORD")
	tenantID := os.Getenv("AZURE_TENANT")

	require.NotEmpty(t, appID, "AZURE_APP_ID must be set")
	require.NotEmpty(t, password, "AZURE_PASSWORD must be set")
	require.NotEmpty(t, tenantID, "AZURE_TENANT must be set")

	cmd := exec.Command("az", "login",
		"--service-principal",
		"--username", appID,
		"--password", password,
		"--tenant", tenantID,
	)

	err := cmd.Run()
	require.NoError(t, err, "Azure login failed")

	t.Log("Azure login successful!")
}

// Setup config (login, load config)
func setupConfig(t *testing.T) (*deployment.Config, string) {

	// Get current working directory
	cwd, err := os.Getwd()
	assert.NoError(t, err)

	// Construct root directory path
	root := filepath.Join(cwd, "..", "..", "..")

	// Set path to config file
	configPath := filepath.Join(root, "azurefunctions_setup", "azurefunctionsconfig.yaml")

	// Perform Azure login
	setupAzureLogin(t)

	// Load config
	config, err := deployment.LoadConfig(configPath)
	require.NoError(t, err, "Failed to load config")

	// Set unique names for Azure Resources
	timestamp := time.Now().Format("150405") // HHMMSS format
	config.AzureConfig.ResourceGroup = fmt.Sprintf("unit-rg-%s", timestamp)
	config.AzureConfig.StorageAccountName = fmt.Sprintf("unitstore%s", timestamp)
	config.AzureConfig.FunctionAppName = fmt.Sprintf("unit-funcapp-%s", timestamp)

	functionAppName := fmt.Sprintf("%s-0", config.AzureConfig.FunctionAppName)
	t.Log("Azure login and config setup completed.")

	return config, functionAppName
}

// Prepare zipped function
func prepareZipFile(t *testing.T) []*common.Function {

	// Get current working directory
	cwd, err := os.Getwd()
	assert.NoError(t, err)

	// Construct root directory path
	root := filepath.Join(cwd, "..", "..", "..")

	baseDir := filepath.Join(root, "azure_functions_for_zip_test")
	sharedWorkloadDir := filepath.Join(root, "azurefunctions_setup", "shared_azure_workload")
	srcPath := filepath.Join(root, "server", "trace-func-py", "exec_func.py")
	dstPath := filepath.Join(sharedWorkloadDir, "exec_func.py")

	functions := []*common.Function{
		{Name: "testfunction0"},
	}

	err = deployment.CopyPythonWorkload(srcPath, dstPath)
	require.NoError(t, err, "Failed to copy exec_func.py")

	err = deployment.CreateFunctionFolders(baseDir, sharedWorkloadDir, functions)
	require.NoError(t, err, "Failed to create function folders")

	err = deployment.ZipFunctionAppFiles(baseDir, functions)
	require.NoError(t, err, "Failed to create zip files")

	t.Log("Zip preparation completed!")

	return functions
}

// Cleanup Azure RG
func cleanupAzureResources(t *testing.T, config *deployment.Config) {

	t.Logf("Cleaning up Resource Group: %s", config.AzureConfig.ResourceGroup)
	err := deployment.DeleteResourceGroup(config)
	assert.NoError(t, err, "Failed to cleanup Resource Group")
}

// Local files cleanup
func cleanupTestArtifacts(t *testing.T, baseDir string, functions []*common.Function) {

	// Remove the copied exec_func.py from shared workload
	execFuncPath := filepath.Join("azurefunctions_setup", "shared_azure_workload", "exec_func.py")
	if err := os.Remove(execFuncPath); err != nil && !os.IsNotExist(err) {
		t.Logf("Warning: failed to remove exec_func.py: %v", err)
	}

	// Remove the test-specific function folders
	if err := os.RemoveAll(baseDir); err != nil && !os.IsNotExist(err) {
		t.Logf("Warning: failed to remove baseDir %s: %v", baseDir, err)
	}

	// Remove the zipped function files (e.g., function0.zip)
	for i := range functions {
		zip := fmt.Sprintf("function%d.zip", i)
		if err := os.Remove(zip); err != nil && !os.IsNotExist(err) {
			t.Logf("Warning: failed to remove zip file %s: %v", zip, err)
		}
	}
}
