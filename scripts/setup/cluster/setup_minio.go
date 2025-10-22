package cluster

import (
	"fmt"
	"path"
	"sync"
	"time"

	"github.com/vhive-serverless/loader/scripts/setup/configs"
	loaderUtils "github.com/vhive-serverless/loader/scripts/setup/utils"
	"github.com/vhive-serverless/vHive/scripts/utils"
)

type MinioConfig struct {
	HelmDownloadUrl string `json:"HelmDownloadUrl"`
	MinIOVersion    string `json:"MinIOVersion"`
	MinIOValuePath  string `json:"MinIOValuePath"`
}

func setupMinio(masterNode string, operatorNode []string, tenantNode []string, minioConfig *configs.MinioConfig) error {
	// Get number of Operator and Tenant node
	numOperator := len(operatorNode)
	numTenant := len(tenantNode)

	_, err := loaderUtils.ServerExec(masterNode, "tmux new -s minio-console -d")
	if !utils.CheckErrorWithMsg(err, "Failed to create tmux session 'minio-console' on master node %s: %v\n", masterNode, err) {
		return err
	}

	// Setup Helm
	err = SetupHelm(masterNode, minioConfig)
	if err != nil {
		return err
	}

	// Create k8s MinIO Namespace
	err = CreateMinioNamespace(masterNode)
	if err != nil {
		return err
	}

	// Install MinIO Operator
	// Add JWT to MinIO console
	err = SetMinioOperator(masterNode, numOperator, minioConfig)
	if err != nil {
		return err
	}

	// Create PV Directory
	// err = CreatePVDir(tenantNode)
	// if err != nil {
	// 	return err
	// }
	err = CreatePVDirC6620(tenantNode, minioConfig)
	if err != nil {
		return err
	}

	// Create PV using helm provisioner
	err = CreateMinioPV(masterNode, tenantNode, minioConfig)
	if err != nil {
		return err
	}

	// Install MinIO Tenant
	err = CreateMinioTenant(masterNode, numTenant, minioConfig)
	if err != nil {
		return err
	}

	err = SetupMinioClient(masterNode)
	if err != nil {
		return err
	}

	time.Sleep(5 * time.Second)

	_, err = loaderUtils.ServerExec(masterNode, `tmux send-keys -t minio-console "while true; do kubectl --namespace minio port-forward svc/myminio-console 9095:9090; done" ENTER`)
	if !utils.CheckErrorWithMsg(err, "Failed to start port-forwarding for MinIO console\n") {
		return err
	}

	return nil
}

func SetupHelm(masterNode string, minioConfig *configs.MinioConfig) error {
	utils.WaitPrintf("Installing Helm\n")
	HelmUrl := minioConfig.HelmDownloadUrl
	_, err := loaderUtils.ServerExec(masterNode, fmt.Sprintf("curl %s | bash", HelmUrl))
	if !utils.CheckErrorWithMsg(err, "Failed to download and install Helm\n") {
		return err
	}

	time.Sleep(1 * time.Second)

	_, err = loaderUtils.ServerExec(masterNode, "helm repo add minio-operator https://operator.min.io")
	if !utils.CheckErrorWithMsg(err, "Failed to add MinIO Repo to Helm\n") {
		return err
	}

	_, err = loaderUtils.ServerExec(masterNode, "helm repo add sig-storage https://kubernetes-sigs.github.io/sig-storage-local-static-provisioner")
	if !utils.CheckErrorWithMsg(err, "Failed to add Sig Storage Repo to Helm\n") {
		return err
	}

	_, err = loaderUtils.ServerExec(masterNode, "helm repo update")
	if !utils.CheckErrorWithMsg(err, "Failed to update Helm repos\n") {
		return err
	}

	return nil
}

func CreateMinioNamespace(masterNode string) error {
	utils.WaitPrintf("Creating MinIO namespace\n")
	_, err := loaderUtils.ServerExec(masterNode, "kubectl create namespace minio")
	if !utils.CheckErrorWithMsg(err, "Failed to create MinIO namespace\n") {
		return err
	}
	return nil
}

func DeleteMinioNamespace(masterNode string) error {
	utils.WaitPrintf("Deleting MinIO namespace\n")
	_, err := loaderUtils.ServerExec(masterNode, "kubectl delete namespace minio")
	if !utils.CheckErrorWithMsg(err, "Failed to delete MinIO namespace\n") {
		return err
	}
	return nil
}

func SetMinioOperator(masterNode string, numOperator int, minioConfig *configs.MinioConfig) error {
	utils.WaitPrintf("Installing MinIO operator\n")
	opConfigPath := path.Join(minioConfig.MinIOValuePath, "minio_operator_values.yaml")
	_, err := loaderUtils.ServerExec(masterNode, fmt.Sprintf("helm upgrade minio-operator --namespace minio minio-operator/operator --install --atomic --version %s -f %s --set operator.replicaCount=%d", minioConfig.MinIOVersion, opConfigPath, numOperator))
	if !utils.CheckErrorWithMsg(err, "Failed to install MinIO operator\n") {
		return err
	}

	bashCmd := `
kubectl apply -f - <<EOF
apiVersion: v1
kind: Secret
metadata:
  name: console-sa-secret
  namespace: minio
  annotations:
    kubernetes.io/service-account.name: console-sa
type: kubernetes.io/service-account-token
EOF
kubectl -n minio get secret console-sa-secret -o jsonpath="{.data.token}" | base64 --decode
`

	// Get the Operator Console URL by running these commands:
	// kubectl --namespace minio port-forward svc/console 9095:9090 due to collision with prometheus
	// echo "Visit the Operator Console at http://127.0.0.1:9095"

	_, err = loaderUtils.ServerExec(masterNode, bashCmd)
	if !utils.CheckErrorWithMsg(err, "Failed to get the JWT for logging in to the console\n") {
		return err
	}

	return nil
}

func UninstallMinioOperator(masterNode string) error {
	utils.WaitPrintf("Uninstalling MinIO operator\n")
	_, err := loaderUtils.ServerExec(masterNode, "helm uninstall minio-operator --namespace minio")
	if !utils.CheckErrorWithMsg(err, "Failed to uninstall MinIO operator\n") {
		return err
	}
	return nil
}

func CreatePVDir(tenantNode []string) error {
	var wg sync.WaitGroup
	errChan := make(chan error, len(tenantNode))

	utils.WaitPrintf("Creating MinIO PV directory on each Tenant node\n")
	// Create PV Directory on each Tenant node
	for _, node := range tenantNode {
		wg.Add(1)
		go func(node string) {
			defer wg.Done()
			_, err := loaderUtils.ServerExec(node, "sudo mkdir -p /mnt/resources/minio")
			if !utils.CheckErrorWithMsg(err, "Failed to create MinIO PV directory on node %s\n", node) {
				errChan <- err
			}
			_, err = loaderUtils.ServerExec(node, "sudo chmod 777 /mnt/resources/minio")
			if !utils.CheckErrorWithMsg(err, "Failed to set permissions for MinIO PV directory on node %s\n", node) {
				errChan <- err
			}
			_, err = loaderUtils.ServerExec(node, "sudo mount --bind /mnt/resources/minio /mnt/resources/minio")
			if !utils.CheckErrorWithMsg(err, "Failed to set permissions for MinIO PV directory on node %s\n", node) {
				errChan <- err
			}
		}(node)
	}
	wg.Wait()
	return nil
}

func CreatePVDirC6620(tenantNode []string, minioConfig *configs.MinioConfig) error {
	var wg sync.WaitGroup
	errChan := make(chan error, len(tenantNode))
	PVDirScript := minioConfig.MinIOValuePath + "/minio_xfs_c6620.sh"

	for _, node := range tenantNode {
		wg.Add(1)
		go func(node string) {
			defer wg.Done()
			utils.InfoPrintf("Creating MinIO PV directory on node %s\n", node)
			_, err := loaderUtils.ServerExec(node, fmt.Sprintf("sudo bash %s", PVDirScript))
			if err != nil {
				errChan <- fmt.Errorf("failed to create MinIO PV directory on node %s: %v", node, err)
			}
		}(node)
	}
	wg.Wait()
	return nil
}

func CleanPVDir(tenantNode []string) error {
	var wg sync.WaitGroup
	errChan := make(chan error, len(tenantNode))

	utils.WaitPrintf("Cleaning MinIO PV directory on each Tenant node\n")
	// Create PV Directory on each Tenant node
	for _, node := range tenantNode {
		wg.Add(1)
		go func(node string) {
			defer wg.Done()
			_, err := loaderUtils.ServerExec(node, "sudo rm -rf /mnt/resources/minio/*")
			if !utils.CheckErrorWithMsg(err, "Failed to remove MinIO PV directory on node %s\n", node) {
				errChan <- err
			}
		}(node)
	}
	wg.Wait()
	return nil
}

func CreateMinioPV(masterNode string, tenantNode []string, minioConfig *configs.MinioConfig) error {
	utils.WaitPrintf("Creating MinIO PV using Helm provisioner\n")
	storageClassPath := path.Join(minioConfig.MinIOValuePath, "minio_storage_class.yaml")
	_, err := loaderUtils.ServerExec(masterNode, fmt.Sprintf("kubectl apply -f %s", storageClassPath))
	if !utils.CheckErrorWithMsg(err, "Failed to create MinIO storage class\n") {
		return err
	}

	tenantPVConfigPath := path.Join(minioConfig.MinIOValuePath, "minio_pv.yaml")
	_, err = loaderUtils.ServerExec(masterNode, fmt.Sprintf("helm upgrade local-provisioner sig-storage/local-static-provisioner --namespace kube-system --install -f %s", tenantPVConfigPath))
	if !utils.CheckErrorWithMsg(err, "Failed to install local static provisioner\n") {
		return err
	}
	return nil
}

func CreateMinioTenant(masterNode string, numTenant int, minioConfig *configs.MinioConfig) error {
	utils.WaitPrintf("Creating MinIO tenant\n")
	tenantConfigPath := path.Join(minioConfig.MinIOValuePath, "minio_tenant_values.yaml")
	_, err := loaderUtils.ServerExec(masterNode, fmt.Sprintf("helm upgrade minio-tenant --namespace minio minio-operator/tenant --install --atomic --version %s -f %s --set tenant.pools[0].servers=%d", minioConfig.MinIOVersion, tenantConfigPath, numTenant))
	if !utils.CheckErrorWithMsg(err, "Failed to create MinIO tenant\n") {
		return err
	}
	return nil
}

func UninstallMinioTenant(masterNode string) error {
	utils.WaitPrintf("Uninstalling MinIO tenant\n")
	_, err := loaderUtils.ServerExec(masterNode, "helm uninstall minio-tenant --namespace minio")
	if !utils.CheckErrorWithMsg(err, "Failed to uninstall MinIO tenant\n") {
		return err
	}
	return nil
}

func SetupMinioClient(masterNode string) error {
	minIOClientUrl := "https://dl.min.io/client/mc/release/linux-amd64/mc"
	utils.WaitPrintf("Setting up MinIO client\n")
	_, err := loaderUtils.ServerExec(masterNode, fmt.Sprintf(`sudo wget %s -O /usr/bin/mc && sudo chmod +x /usr/bin/mc`, minIOClientUrl))
	if !utils.CheckErrorWithMsg(err, "Failed to set up MinIO client\n") {
		return err
	}
	return nil
}

func DeletePVC(masterNode string, minioConfig *configs.MinioConfig) error {
	utils.WaitPrintf("Deleting MinIO PVC\n")
	_, err := loaderUtils.ServerExec(masterNode, "helm uninstall local-provisioner --namespace kube-system")
	if !utils.CheckErrorWithMsg(err, "Failed to delete MinIO PV\n") {
		return err
	}

	storageClassPath := path.Join(minioConfig.MinIOValuePath, "minio_storage_class.yaml")
	_, err = loaderUtils.ServerExec(masterNode, fmt.Sprintf("kubectl delete -f %s", storageClassPath))
	if !utils.CheckErrorWithMsg(err, "Failed to delete MinIO storage class\n") {
		return err
	}

	_, err = loaderUtils.ServerExec(masterNode, "kubectl delete pvc --all -n minio")
	if !utils.CheckErrorWithMsg(err, "Failed to delete MinIO PVCs\n") {
		return err
	}

	_, err = loaderUtils.ServerExec(masterNode, "kubectl delete pv $(kubectl get pv | grep '^local-pv-' | awk '{print $1}')")
	utils.CheckErrorWithMsg(err, "Failed to delete MinIO PVs\n")

	return nil
}

func CleanupMinio(configDir string, configName string) error {
	cfg, err := configs.CommonConfigSetup(configDir, configName)
	if err != nil {
		utils.FatalPrintf("Failed to load configurations: %v\n", err)
		return err
	}

	// Uninstall MinIO Tenant
	UninstallMinioTenant(cfg.MasterNode)

	// Uninstall MinIO Operator
	UninstallMinioOperator(cfg.MasterNode)

	// Delete PVCs
	DeletePVC(cfg.MasterNode, cfg.MinioConfig)

	// Delete MinIO Namespace
	DeleteMinioNamespace(cfg.MasterNode)

	// Clean PV Directory on Tenant nodes
	CleanPVDir(cfg.MinioTenantNodes)

	time.Sleep(5 * time.Second)

	return nil
}

func RedeployMinio(configDir string, configName string) error {
	cfg, err := configs.CommonConfigSetup(configDir, configName)
	if err != nil {
		utils.FatalPrintf("Failed to load configurations: %v\n", err)
		return err
	}
	numOperator := len(cfg.MinioOperatorNodes)
	numTenant := len(cfg.MinioTenantNodes)

	// Create k8s MinIO Namespace
	err = CreateMinioNamespace(cfg.MasterNode)
	if err != nil {
		return err
	}

	err = SetMinioOperator(cfg.MasterNode, numOperator, cfg.MinioConfig)
	if err != nil {
		return err
	}

	// Create PV using helm provisioner
	err = CreateMinioPV(cfg.MasterNode, cfg.MinioTenantNodes, cfg.MinioConfig)
	if err != nil {
		return err
	}

	// Install MinIO Tenant
	err = CreateMinioTenant(cfg.MasterNode, numTenant, cfg.MinioConfig)
	if err != nil {
		return err
	}

	time.Sleep(5 * time.Second)

	return nil
}
