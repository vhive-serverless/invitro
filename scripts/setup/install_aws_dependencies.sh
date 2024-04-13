#!/usr/bin/env bash

#
# MIT License
#
# Copyright (c) 2024 HySCALE
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

LOADER_NODE=$1
GO_VERSION=1.21.9

server_exec() {
  ssh -oStrictHostKeyChecking=no -p 22 $LOADER_NODE $1;
}

# Check the architecture
arch=`server_exec 'uname -m'`
case $arch in
  'x86_64')
    arch='amd64'
    ;;
  'aarch64')
    arch='arm64'
    ;;
  *)
    echo "Unsupported architecture $arch"
    exit 1
    ;;
esac

# Obtain configuration variables
DIR=$(cd "$(dirname ${BASH_SOURCE[0]})" > /dev/null 2>&1 && pwd)
source "$DIR/setup.cfg"

echo "Installing the dependencies for AWS deployment"

# ========== Get loader ==========
server_exec "git clone --depth=1 --branch=$LOADER_BRANCH https://github.com/vhive-serverless/invitro.git loader"
echo "Installed the Github repository for the loader"

# ========== Install AWS CLI ==========
server_exec 'curl "https://awscli.amazonaws.com/awscli-exe-linux-$(uname -m).zip" -o /tmp/awscliv2.zip'
server_exec 'unzip -q /tmp/awscliv2.zip -d /tmp'
server_exec 'rm /tmp/awscliv2.zip'
server_exec 'sudo /tmp/aws/install --update'
server_exec 'rm -rf /tmp/aws/'
echo "Installed AWS CLI"

# ========== Install Docker ==========
# Add Docker's official GPG key:
server_exec 'sudo apt-get update'
server_exec 'sudo apt-get install ca-certificates curl'
server_exec 'sudo install -m 0755 -d /etc/apt/keyrings'
server_exec 'sudo curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc'
server_exec 'sudo chmod a+r /etc/apt/keyrings/docker.asc'
echo "Added Docker's official GPG key"

# Add the repository to Apt sources:
server_exec 'echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/ubuntu $(. /etc/os-release && echo "$VERSION_CODENAME") stable" | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null'
server_exec 'sudo apt-get update'
echo "Added the repository to Apt sources"

# To install the latest Docker version, run:
server_exec 'sudo apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin'

# To use Docker as a non-root user, adding user to the docker group:
server_exec 'sudo usermod -aG docker ${USER}'
echo "Installed Docker"

# ========== Install golang ==========
server_exec "curl -L 'https://golang.org/dl/go$GO_VERSION.linux-$arch.tar.gz' -o /tmp/go.tar.gz"
server_exec 'sudo rm -rf /usr/local/go && sudo tar -C /usr/local -xzf /tmp/go.tar.gz'
server_exec 'sudo rm -f /tmp/go.tar.gz'
server_exec 'echo "export PATH=\$PATH:/usr/local/go/bin" >> ~/.profile'
echo "Installed golang"

# ========== Install serverless.com framework (as a standalone binary without node.js / npm dependencies) ==========
server_exec 'curl -o- -L https://slss.io/install | VERSION=3.34.0 bash'
# Do not require export PATH as the installation comes with it
echo "Installed serverless.com framework"

# ========== Check the installed versions ==========
echo "Checking the installed versions:"
server_exec 'source ~/.profile; aws --version'
server_exec 'source ~/.profile; docker --version'
server_exec 'source ~/.profile; serverless --version'
server_exec 'source ~/.profile; go version'

echo "Finished installing the dependencies for AWS deployment"