#!/bin/bash

# Create ssh keys
echo "Creating ssh keys"
ssh-keygen -t rsa -N "" -f ~/.ssh/id_rsa > /dev/null
chmod 600 ~/.ssh/id_rsa
eval "$(ssh-agent -s)" && ssh-add

# Store pub key and username in kubectl secrets
echo "Storing ssh keys in kubectl secrets"
cat <<EOF > ssh_pub.yaml
apiVersion: v1
kind: Secret
metadata:
  name: sshpub
type: Opaque
data:
  pubkey: $(cat ~/.ssh/id_rsa.pub | base64 | tr -d '\n')
EOF

kubectl create -f ssh_pub.yaml

# Install ssh server in target pod
echo "Installing ssh server in target pod"
docker exec knative-control-plane apt-get update
docker exec knative-control-plane apt-get install -y openssh-server
## Install other packages
docker exec knative-control-plane apt-get install -y psmisc
# Edit sshd_config
echo "Editing sshd_config"
docker exec knative-control-plane sed -i 's/^#\?PasswordAuthentication.*/PasswordAuthentication no/' /etc/ssh/sshd_config
docker exec knative-control-plane sed -i 's/^#\?AuthorizedKeysFile.*/AuthorizedKeysFile \.ssh\/authorized_keys/' /etc/ssh/sshd_config
docker exec knative-control-plane sed -i 's/^#\?PubkeyAuthentication.*/PubkeyAuthentication yes/' /etc/ssh/sshd_config
docker exec knative-control-plane systemctl restart sshd

# Add user 
echo "Adding user"
docker exec knative-control-plane adduser --disabled-password --allow-bad-names --gecos "" $(whoami)

# Add keys to user's authorized_keys
echo "Adding keys to authorized_keys"
docker exec knative-control-plane mkdir /home/$(whoami)/.ssh/ 
docker exec knative-control-plane touch /home/$(whoami)/.ssh/authorized_keys
docker exec knative-control-plane sh -c "kubectl get secrets sshpub -o jsonpath='{.data.pubkey}' | base64 -d >> /home/$(whoami)/.ssh/authorized_keys"

docker exec knative-control-plane chmod 700 /home/$(whoami)/.ssh
docker exec knative-control-plane chmod 600 /home/$(whoami)/.ssh/authorized_keys
docker exec knative-control-plane chown -R $(whoami):$(whoami) /home/$(whoami)/.ssh

# Clean up
rm ssh_pub.yaml

# Update /var user group and permission
docker exec knative-control-plane chown -R $(whoami):$(whoami) /var
docker exec knative-control-plane chmod -R 777 /var

# Make user sudo
docker exec knative-control-plane sh -c "apt-get update && apt-get install -y sudo"
docker exec knative-control-plane usermod -aG sudo $(whoami)
docker exec knative-control-plane sh -c "echo \"$(whoami) ALL=(ALL) NOPASSWD: ALL\" > /etc/sudoers.d/$(whoami)"
