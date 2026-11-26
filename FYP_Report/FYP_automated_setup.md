### Renting CloudLab Nodes
[Renting with vHive Profile](https://www.cloudlab.us/p/ntu-cloud/vhive-ubuntu24)

### Connecting to CloudLab Nodes
`ssh -o ServerAliveInterval=60 -A bryanFB@hp142.utah.cloudlab.us` (Master)
`ssh -o ServerAliveInterval=60 -A bryanFB@hp021.utah.cloudlab.us` (Loader)

LIST OF NODES
ssh bryanFB@hp142.utah.cloudlab.us
ssh bryanFB@hp021.utah.cloudlab.us

ssh bryanFB@hp125.utah.cloudlab.us
ssh bryanFB@hp126.utah.cloudlab.us
ssh bryanFB@hp152.utah.cloudlab.us
ssh bryanFB@hp131.utah.cloudlab.us
ssh bryanFB@hp130.utah.cloudlab.us
ssh bryanFB@hp015.utah.cloudlab.us
ssh bryanFB@hp032.utah.cloudlab.us
ssh bryanFB@hp122.utah.cloudlab.us
ssh bryanFB@hp146.utah.cloudlab.us
ssh bryanFB@hp149.utah.cloudlab.us
ssh bryanFB@hp132.utah.cloudlab.us
ssh bryanFB@hp160.utah.cloudlab.us
ssh bryanFB@hp139.utah.cloudlab.us
ssh bryanFB@hp121.utah.cloudlab.us
ssh bryanFB@hp143.utah.cloudlab.us
ssh bryanFB@hp158.utah.cloudlab.us
ssh bryanFB@hp151.utah.cloudlab.us


Update Node list on 
- setup_master.sh
- setup_loader.sh
- the commands in this markdown file

## Master Node Setup
Copy over script from Windows
`scp -o "StrictHostKeyChecking=accept-new" -r "D:\FYP\Projects\Actual_Invitro_Development\remote_setup" bryanFB@hp142.utah.cloudlab.us:~/` (Laptop)

Execute script on master node
`ssh -o ServerAliveInterval=60 -A bryanFB@hp142.utah.cloudlab.us`

`tmux new -s work` New session 
`cd ~/remote_setup && chmod u+x ./setup_master.sh && ./setup_master.sh`
`tmux a -t work` Attach

## Loader Node Setup
Copy over script from Windows
`scp -o "StrictHostKeyChecking=accept-new" -r "D:\FYP\Projects\Actual_Invitro_Development\remote_setup" bryanFB@hp021.utah.cloudlab.us:~/` (Laptop)

Execute script on loader node
`ssh -o ServerAliveInterval=60 -A bryanFB@hp021.utah.cloudlab.us`

`tmux new -s work` New session 
`cd ~/remote_setup && chmod u+x ./setup_loader.sh && ./setup_loader.sh`
`tmux a -t work` Attach

## Refresh Token
`rclone config reconnect remote:`
Ubuntu WSL is sufficient, portforwarding.

## Performing Experiments
Update multiple_runs.sh
- Update all the variables
- Create new test_jsons (update output path and input folders)

tmux 
`tmux new -s work` New session 
`Ctrl + b` `d` Detach
`tmux a -t work` Attach

Execute job script
`cd ~/loader`
`chmod +x multiple_runs.sh`
`./multiple_runs.sh`

## Post Experiment Logging
`sudo journalctl -u kubelet --since "2026-11-19 12:00" --until "2026-11-19 12:45" | grep -iE "apiserver|etcd|liveness|unhealthy|oom"`
`sudo dmesg -T | grep -iE "oom|killed process"`
`sudo crictl ps -a | grep -E "kube-apiserver|etcd"`
`sudo crictl logs <container-id> 2>&1 | tail -200`
`sudo ls -la /var/log/pods/ | grep -E "apiserver|etcd"`