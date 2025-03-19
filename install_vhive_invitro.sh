git clone --depth=1 https://github.com/vhive-serverless/vhive.git
cd vhive
mkdir -p /tmp/vhive-logs

./scripts/install_go.sh; source /etc/profile
pushd scripts && go build -o setup_tool && popd

./scripts/setup_tool setup_node stock-only
sudo screen -d -m containerd
./scripts/setup_tool create_one_node_cluster stock-only

cd ..
git clone --branch=sesame25-tutorial https://github.com/vhive-serverless/invitro.git
kubectl patch configmap/config-autoscaler -n knative-serving -p '{"data":{"allow-zero-initial-scale":"true"}}'
kubectl patch configmap/config-autoscaler -n knative-serving -p '{"data":{"initial-scale":"0"}}'

pushd invitro
git lfs install
git lfs fetch
git lfs checkout
sudo apt install -y pip
pip install -r requirements.txt
wget https://azurepublicdatasettraces.blob.core.windows.net/azurepublicdatasetv2/azurefunctions_dataset2019/azurefunctions-dataset2019.tar.xz -P ./data/azure
pushd data/azure
tar -xvf azurefunctions-dataset2019.tar.xz
popd