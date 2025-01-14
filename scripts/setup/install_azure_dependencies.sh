#!/usr/bin/env bash

GO_VERSION=1.23.0

# Check the architecture
arch=$(uname -m)
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

echo "Installing dependencies for Azure deployment..."

# ========== Install Azure CLI ==========
curl -sL https://aka.ms/InstallAzureCLIDeb | sudo bash
if [ $? -eq 0 ]; then
  echo "Azure CLI installed successfully"
else
  echo "Failed to install Azure CLI"
  exit 1
fi

# ========== Install golang ==========
curl -L "https://golang.org/dl/go$GO_VERSION.linux-$arch.tar.gz" -o /tmp/go.tar.gz
sudo rm -rf /usr/local/go && sudo tar -C /usr/local -xzf /tmp/go.tar.gz
rm -f /tmp/go.tar.gz
echo "export PATH=\$PATH:/usr/local/go/bin" >> ~/.profile
export PATH=$PATH:/usr/local/go/bin

if command -v go &> /dev/null; then
  echo "Go installed successfully"
else
  echo "Failed to install Go"
  exit 1
fi

# ========== Check the installed versions ==========
echo "Checking installed versions:"
az --version
go version

echo "Finished installing Azure CLI and Go"