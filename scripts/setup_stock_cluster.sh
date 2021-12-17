#!/usr/bin/env bash
# Spin up vHive under container mode.

if [ -z $1 ]; then
    BRANCH="main"
fi

cd 
git clone --branch=$BRANCH https://github.com/ease-lab/vhive
cd vhive
./scripts/cloudlab/setup_node.sh stock-only
tmux new -d -s containerd
tmux new -d -s cluster
tmux send-keys -t containerd "sudo containerd" ENTER
sleep 3
tmux send-keys -t cluster "watch -n 0.5 kubectl get pods -A" ENTER

# Update golang.
wget https://dl.google.com/go/go1.17.linux-amd64.tar.gz
sudo rm -rf /usr/local/go && sudo tar -C /usr/local/ -xzf go1.17.linux-amd64.tar.gz
rm go1.17*
echo "export PATH=$PATH:/usr/local/go/bin" >> ~/.profile
source ~/.profile