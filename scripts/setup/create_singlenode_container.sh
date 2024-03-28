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

    server_exec "pushd ~/vhive/scripts > /dev/null && ./install_go.sh && source /etc/profile && go build -o setup_tool && ./setup_tool setup_node stock-only && popd > /dev/null"

    # Get loader and dependencies.
    server_exec "git clone --branch=$LOADER_BRANCH https://github.com/vhive-serverless/invitro.git loader"
    server_exec 'echo -en "\n\n" | sudo apt-get install python3-pip python-dev'
    server_exec 'cd; cd loader; pip install -r config/requirements.txt'
    
    server_exec '~/loader/scripts/setup/rewrite_yaml_files.sh single_node'
    
    server_exec 'tmux new -s runner -d'
    server_exec 'tmux new -s kwatch -d'
    server_exec 'tmux new -d -s containerd'
    server_exec 'tmux new -d -s cluster'
    server_exec 'tmux send-keys -t containerd "sudo containerd" ENTER'
    sleep 3
    server_exec 'pushd ~/vhive/scripts > /dev/null && ./setup_tool create_one_node_cluster stock-only && popd > /dev/null'
    server_exec 'tmux send-keys -t cluster "watch -n 0.5 kubectl get pods -A" ENTER'

    if [[ "$DEPLOY_PROMETHEUS" == true ]]; then
        $DIR/expose_infra_metrics.sh $SERVER single_node
    fi

    # Stabilize the node
    server_exec '~/loader/scripts/setup/stabilize.sh'
}
