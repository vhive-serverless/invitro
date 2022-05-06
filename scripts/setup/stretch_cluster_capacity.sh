echo "maxPods: 240" > >(sudo tee -a /var/lib/kubelet/config.yaml >/dev/null)
sudo systemctl restart kubelet