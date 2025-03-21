git clone --depth=1 https://github.com/vhive-serverless/vhive.git
cd vhive
mkdir -p /tmp/vhive-logs

# ./scripts/install_go.sh; source /etc/profile
# pushd scripts && go build -o setup_tool && popd

pushd ~/vhive/scripts > /dev/null && ./install_go.sh && source /etc/profile && go build -o setup_tool && ./setup_tool setup_node stock-only && popd > /dev/null

# ./scripts/setup_tool setup_node stock-only
# sudo screen -d -m containerd
# ./scripts/setup_tool create_one_node_cluster stock-only

cd ..
git clone --branch=sesame25-tutorial https://github.com/vhive-serverless/invitro.git loader
echo -en "\n\n" | sudo apt-get install python3-pip python-dev
cd; cd loader; pip install -r config/requirements.txt


~/loader/scripts/setup/rewrite_yaml_files.sh
tmux new -s runner -d
tmux new -s kwatch -d
tmux new -d -s containerd
tmux new -d -s cluster
tmux send-keys -t containerd "sudo containerd" ENTER
sleep 3

pushd ~/vhive/scripts > /dev/null && ./setup_tool prepare_one_node_cluster stock-only && popd > /dev/null
kubectl label nodes --all loader-nodetype=singlenode
pushd ~/vhive/scripts > /dev/null && ./setup_tool setup_master_node stock-only && popd > /dev/null

tmux send-keys -t cluster "watch -n 0.5 kubectl get pods -A" ENTER
kubectl patch configmap -n knative-serving config-features -p '{\"data\": {\"kubernetes.podspec-affinity\": \"enabled\"}}'
cd loader; bash scripts/setup/patch_init_scale.sh

~/loader/scripts/setup/stabilize.sh


# kubectl patch configmap/config-autoscaler -n knative-serving -p '{"data":{"allow-zero-initial-scale":"true"}}'
# kubectl patch configmap/config-autoscaler -n knative-serving -p '{"data":{"initial-scale":"0"}}'

pushd loader
git lfs install
git lfs fetch
git lfs checkout
# sudo apt install -y pip
# pip install -r requirements.txt
wget https://azurepublicdatasettraces.blob.core.windows.net/azurepublicdatasetv2/azurefunctions_dataset2019/azurefunctions-dataset2019.tar.xz -P ./data/azure
pushd data/azure
tar -xvf azurefunctions-dataset2019.tar.xz
popd