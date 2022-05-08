sudo iptables -P FORWARD ACCEPT
sudo iptables -I FORWARD 1 -i cni0 -j ACCEPT -m comment --comment "flannel subnet"
sudo iptables -I FORWARD 1 -o cni0 -j ACCEPT -m comment --comment "flannel subnet"
sudo iptables -t nat -A POSTROUTING -s 10.244.0.0/16 ! -d 10.244.0.0/16 -j MASQUERADE
