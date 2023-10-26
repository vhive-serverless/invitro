#!/usr/bin/env bash
#
# MIT License
#
# Copyright (c) 2023 EASL and the vHive community
#
# Permission is hereby granted, free of charge, to any person obtaining a copy
# of this software and associated documentation files (the "Software"), to deal
# in the Software without restriction, including without limitation the rights
# to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
# copies of the Software, and to permit persons to whom the Software is
# furnished to do so, subject to the following conditions:
#
# The above copyright notice and this permission notice shall be included in all
# copies or substantial portions of the Software.
#
# THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
# IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
# FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
# AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
# LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
# OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
# SOFTWARE.
#

SERVER=$1

DIR="$(pwd)/scripts/setup/"

source "$(pwd)/scripts/setup/setup.cfg"

server_exec() { 
    ssh -oStrictHostKeyChecking=no -p 22 "$SERVER" $1; 
}

{
    # Spin up vHive under container mode.
    server_exec 'sudo DEBIAN_FRONTEND=noninteractive apt-get autoremove' 
    server_exec "git clone --branch=$VHIVE_BRANCH https://github.com/ease-lab/vhive"
    server_exec 'cd vhive; ./scripts/cloudlab/setup_node.sh stock-only'
    server_exec 'tmux new -s runner -d'
    server_exec 'tmux new -s kwatch -d'
    server_exec 'tmux new -d -s containerd'
    server_exec 'tmux new -d -s cluster'
    server_exec 'tmux send-keys -t containerd "sudo containerd" ENTER'
    sleep 3
    server_exec 'cd vhive; ./scripts/cluster/create_one_node_cluster.sh stock-only'
    server_exec 'tmux send-keys -t cluster "watch -n 0.5 kubectl get pods -A" ENTER'

    # Update golang.
    server_exec 'wget -q https://dl.google.com/go/go1.17.linux-amd64.tar.gz'
    server_exec 'sudo rm -rf /usr/local/go && sudo tar -C /usr/local/ -xzf go1.17.linux-amd64.tar.gz'
    server_exec 'rm go1.17*'
    server_exec 'echo "export PATH=$PATH:/usr/local/go/bin" >> ~/.profile'
    server_exec 'source ~/.profile'

    # Setup github authentication.
    ACCESS_TOKEH="$(cat $GITHUB_TOKEN)"

    server_exec 'echo -en "\n\n" | ssh-keygen -t rsa'
    server_exec 'ssh-keyscan -t rsa github.com >> ~/.ssh/known_hosts'
    # server_exec 'RSA=$(cat ~/.ssh/id_rsa.pub)'

    server_exec 'curl -H "Authorization: token '"$ACCESS_TOKEH"'" --data "{\"title\":\"'"key:\$(hostname)"'\",\"key\":\"'"\$(cat ~/.ssh/id_rsa.pub)"'\"}" https://api.github.com/user/keys'
    # server_exec 'sleep 5'

    # Get loader and dependencies.
    server_exec "git clone --branch=$LOADER_BRANCH https://github.com/vhive-serverless/invitro.git loader"
    server_exec 'echo -en "\n\n" | sudo apt-get install python3-pip python-dev'
    server_exec 'cd; cd loader; pip install -r config/requirements.txt'

    $DIR/expose_infra_metrics.sh $SERVER

    # Stabilize the node
    server_exec './vhive/scripts/stabilize.sh'
}
