package deployment_test

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"

	"github.com/stretchr/testify/assert"
	"github.com/vhive-serverless/loader/pkg/common"
	"github.com/vhive-serverless/loader/pkg/driver/deployment"
)

// mockExecCommand simulates exec.Command for testing
func mockExecCommand(output string, err error) deployment.CommandRunner {
	return func(name string, arg ...string) *exec.Cmd {
		cmd := exec.Command("echo", output) // Simulate success
		if err != nil {
			return exec.Command("false") // Simulate failure
		}
		return cmd
	}
}

// Tests CopyPythonWorkload function correctly copies the exec_func.py file and verifies the contents.
func TestCopyPythonWorkload(t *testing.T) {
	// Change the working directory to project root
	err := os.Chdir("../../../")
	if err != nil {
		t.Fatalf("Failed to change working directory: %s", err)
	}

	// Log current working directory for debugging
	wd, err := os.Getwd()
	assert.NoError(t, err, "Failed to get current working directory")
	t.Logf("Current working directory: %s", wd)

	// Define the actual source path
	srcPath := "server/trace-func-py/exec_func.py"

	// Read the actual content of the source file
	srcContent, err := os.ReadFile(srcPath)
	assert.NoError(t, err)

	// Define the actual destination path
	dstPath := "azurefunctions_setup/shared_azure_workload/exec_func.py"

	// Call the copyPythonWorkload function
	err = deployment.CopyPythonWorkload(srcPath, dstPath)
	assert.NoError(t, err)

	// Verify that the file was copied correctly
	dstContent, err := os.ReadFile(dstPath)
	assert.NoError(t, err)
	assert.Equal(t, string(srcContent), string(dstContent))

}

// Tests zip file contains the correct files and directory structure.
func TestZipHealth(t *testing.T) {
	// Define paths and expected structure
	zipFilePath := "azurefunctions.zip"
	baseDir := "azure_functions_for_zip"
	expectedFunctionCount := 2 // Update if needed

	expectedFunctionFiles := []string{
		"azurefunctionsworkload.py",
		"exec_func.py",
		"function.json",
	}

	expectedRootFiles := []string{
		"requirements.txt",
		"host.json",
	}

	// Create test functions to simulate a real deployment
	functions := []*common.Function{
		{Name: "function0"},
		{Name: "function1"},
	}

	// Change the working directory to project root
	err := os.Chdir("../../../")
	if err != nil {
		t.Fatalf("Failed to change working directory: %s", err)
	}

	// Log current working directory for debugging
	wd, err := os.Getwd()
	assert.NoError(t, err, "Failed to get current working directory")
	t.Logf("Current working directory: %s", wd)

	// Step 1: Create function folders
	err = deployment.CreateFunctionFolders(baseDir, functions)
	assert.NoError(t, err, "Failed to create function folders")

	// Step 2: Zip the function app files
	err = deployment.ZipFunctionAppFiles()
	assert.NoError(t, err, "Failed to create function app zip")

	// Step 3: Validate if zip file exists
	if _, err := os.Stat(zipFilePath); os.IsNotExist(err) {
		t.Fatalf("Zip file does not exist: %s", zipFilePath)
	}

	// Step 4: Open zip file
	r, err := zip.OpenReader(zipFilePath)
	assert.NoError(t, err, "Failed to open zip file")
	defer r.Close()

	// Step 5: Prepare expected files map
	expectedFiles := make(map[string]bool)

	for i := 0; i < expectedFunctionCount; i++ {
		functionFolder := fmt.Sprintf("function%d/", i)
		for _, file := range expectedFunctionFiles {
			expectedFiles[functionFolder+file] = false
		}
	}
	for _, file := range expectedRootFiles {
		expectedFiles[file] = false
	}

	// Step 6: Check the files inside the zip
	for _, f := range r.File {
		filePath := f.Name
		filePath = strings.TrimPrefix(filePath, "./") // Normalize path

		if _, exists := expectedFiles[filePath]; exists {
			expectedFiles[filePath] = true
		}
	}

	// Step 7: Ensure all expected files are present
	for file, found := range expectedFiles {
		assert.True(t, found, "Missing expected file in zip: "+file)
	}

	t.Log("Zip file structure validation passed!")

	// Cleanup: Remove the created files and directories after the test
	err = os.RemoveAll(baseDir)
	assert.NoError(t, err, "Failed to remove temp directory")
	err = os.Remove(zipFilePath)
	assert.NoError(t, err, "Failed to remove temp zip file")
}

// Tests DeployFunction function by simulating deployment of zip file to Azure.
func TestDeployFunction(t *testing.T) {
	// Mock command runner that simulates successful deployment
	mockRunner := mockExecCommand("mocked success", nil)

	// Test configuration
	cfg := &deployment.Config{
		AzureConfig: struct {
			ResourceGroup      string "yaml:\"resource_group\""
			StorageAccountName string "yaml:\"storage_account_name\""
			FunctionAppName    string "yaml:\"function_app_name\""
			Location           string "yaml:\"location\""
		}{
			ResourceGroup:      "test-group",
			StorageAccountName: "test-storage",
			FunctionAppName:    "test-app",
			Location:           "eastus",
		},
	}

	// Mock function list
	functions := []*common.Function{
		{Name: "function0"},
		{Name: "function1"},
	}

	// Capture log output
	var logBuffer bytes.Buffer
	logrus.SetOutput(&logBuffer)
	logrus.SetFormatter(&logrus.TextFormatter{}) // Ensure logs are formatted correctly
	logrus.SetLevel(logrus.InfoLevel)            // Capture INFO logs

	// Run the function
	err := deployment.DeployFunction(cfg, functions, mockRunner)
	assert.NoError(t, err, "Expected deployment to succeed")

	// Verify function endpoints were set correctly
	assert.Equal(t, "https://test-app.azurewebsites.net/api/function0", functions[0].Endpoint)
	assert.Equal(t, "https://test-app.azurewebsites.net/api/function1", functions[1].Endpoint)

	// Check log output
	logs := logBuffer.String()
	assert.Contains(t, logs, "Deploying 2 functions to Azure Function App")
	assert.Contains(t, logs, "Deployed all 2 functions successfully")
}

// Tests CleanUpDeploymentFiles function correctly removes the specified temporary files and directories.
func TestCleanup(t *testing.T) {
	// Setup: Create temporary files and directories
	err := os.Mkdir("azure_functions_for_zip", 0755)
	assert.NoError(t, err, "Failed to create temp directory")

	err = os.WriteFile("azurefunctions.zip", []byte("test content"), 0644)
	assert.NoError(t, err, "Failed to create temp zip file")

	err = deployment.CleanUpDeploymentFiles("azure_functions_for_zip", "azurefunctions.zip")
	assert.NoError(t, err, "Cleanup function returned an error")

	// Verify Cleanup
	t.Log("Verifying cleanup of temp local files...")

	// Check that temporary files and folders are removed
	_, err = os.Stat("azure_functions_for_zip")
	assert.True(t, os.IsNotExist(err), "Temp directory should be deleted")
	_, err = os.Stat("azurefunctions.zip")
	assert.True(t, os.IsNotExist(err), "Temp zip file should be deleted")

	t.Log("Cleaning of local files passed!")

	// Teardown: Clean up any remaining files or directories
	_ = os.RemoveAll("azure_functions_for_zip")
	_ = os.Remove("azurefunctions.zip")
}
