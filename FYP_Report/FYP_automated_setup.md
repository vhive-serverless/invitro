### Renting CloudLab Nodes
[Renting with vHive Profile](https://www.cloudlab.us/p/ntu-cloud/vhive-ubuntu24)

### Connecting to CloudLab Nodes
`ssh -o ServerAliveInterval=60 -A bryanFB@pc475.emulab.net` (Master)
`ssh -o ServerAliveInterval=60 -A bryanFB@pc505.emulab.net` (Loader)

LIST OF NODES
ssh bryanFB@pc475.emulab.net
ssh bryanFB@pc505.emulab.net

ssh bryanFB@pc443.emulab.net
ssh bryanFB@pc444.emulab.net
ssh bryanFB@pc427.emulab.net
ssh bryanFB@pc554.emulab.net
ssh bryanFB@pc434.emulab.net
ssh bryanFB@pc506.emulab.net
ssh bryanFB@pc544.emulab.net
ssh bryanFB@pc504.emulab.net
ssh bryanFB@pc431.emulab.net
ssh bryanFB@pc438.emulab.net
ssh bryanFB@pc528.emulab.net
ssh bryanFB@pc417.emulab.net

Update Node list on 
- setup_master.sh
- setup_loader.sh
- the commands in this markdown file

## Master Node Setup
Copy over script from Windows
`scp -o "StrictHostKeyChecking=accept-new" -r "D:\FYP\Projects\Actual_Invitro_Development\remote_setup" bryanFB@pc475.emulab.net:~/` (Laptop)

Execute script on master node
`ssh -o ServerAliveInterval=60 -A bryanFB@pc475.emulab.net`

`tmux new -s work` New session 
`cd ~/remote_setup && chmod u+x ./setup_master.sh && ./setup_master.sh`
`tmux a -t work` Attach

## Loader Node Setup
Copy over script from Windows
`scp -o "StrictHostKeyChecking=accept-new" -r "D:\FYP\Projects\Actual_Invitro_Development\remote_setup" bryanFB@pc505.emulab.net:~/` (Laptop)

Execute script on loader node
`ssh -o ServerAliveInterval=60 -A bryanFB@pc505.emulab.net`

`tmux new -s work` New session 
`cd ~/remote_setup && chmod u+x ./setup_loader.sh && ./setup_loader.sh`
`tmux a -t work` Attach


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