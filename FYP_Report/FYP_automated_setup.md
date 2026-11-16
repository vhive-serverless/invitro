### Renting CloudLab Nodes
[Renting with vHive Profile](https://www.cloudlab.us/p/ntu-cloud/vhive-ubuntu24)

### Connecting to CloudLab Nodes
`ssh -o ServerAliveInterval=60 -A bryanFB@pc737.emulab.net` (Master)
`ssh -o ServerAliveInterval=60 -A bryanFB@pc815.emulab.net` (Loader)

LIST OF NODES

Update Node list on 
- setup_master.sh
- setup_loader.sh
- the commands in this markdown file

## Master Node Setup
Copy over script from Windows
`scp -o "StrictHostKeyChecking=accept-new" -r "D:\FYP\Projects\Actual_Invitro_Development\remote_setup" bryanFB@pc722.emulab.net:~/` (Home PC)

Execute script on master node
`chmod u+x setup_master.sh`
`./setup_master.sh`

## Loader Node Setup
Copy over script from Windows
`scp -o "StrictHostKeyChecking=accept-new" -r "D:\FYP\Projects\Actual_Invitro_Development\remote_setup" bryanFB@pc815.emulab.net:~/` (Home PC)

Execute script on loader node
`ssh -o ServerAliveInterval=60 -A bryanFB@pc815.emulab.net`
`cd ~/remote_setup`
`chmod u+x setup_loader.sh`
`./setup_loader.sh`

## Performing Experiments
Update multiple_runs.sh
- Update all the variables
- Create new test_jsons (update output path and input folders)

tmux 
`tmux new -s work` New session 
`Ctrl + b` `d` Detach
`tmux a -t work` Attach

Execute job script
`chmod +x multiple_runs.sh`
`./multiple_runs.sh`
