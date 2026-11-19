### Renting CloudLab Nodes
[Renting with vHive Profile](https://www.cloudlab.us/p/ntu-cloud/vhive-ubuntu24)

### Connecting to CloudLab Nodes
`ssh -o ServerAliveInterval=60 -A bryanFB@apt164.apt.emulab.net` (Master)
`ssh -o ServerAliveInterval=60 -A bryanFB@apt156.apt.emulab.net` (Loader)

LIST OF NODES
ssh bryanFB@apt164.apt.emulab.net
ssh bryanFB@apt156.apt.emulab.net

ssh bryanFB@apt130.apt.emulab.net
ssh bryanFB@apt132.apt.emulab.net
ssh bryanFB@apt131.apt.emulab.net
ssh bryanFB@apt179.apt.emulab.net
ssh bryanFB@apt177.apt.emulab.net
ssh bryanFB@apt136.apt.emulab.net

Update Node list on 
- setup_master.sh
- setup_loader.sh
- the commands in this markdown file

## Master Node Setup
Copy over script from Windows
`scp -o "StrictHostKeyChecking=accept-new" -r "D:\FYP\Projects\Actual_Invitro_Development\remote_setup" bryanFB@apt164.apt.emulab.net:~/` (Laptop)

Execute script on master node
`ssh -o ServerAliveInterval=60 -A bryanFB@apt164.apt.emulab.net`

`cd ~/remote_setup`
`chmod u+x ./setup_master.sh`
`./setup_master.sh`

## Loader Node Setup
Copy over script from Windows
`scp -o "StrictHostKeyChecking=accept-new" -r "D:\FYP\Projects\Actual_Invitro_Development\remote_setup" bryanFB@apt156.apt.emulab.net:~/` (Laptop)

Execute script on loader node
`ssh -o ServerAliveInterval=60 -A bryanFB@apt156.apt.emulab.net`

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
`cd ~/loader`
`chmod +x multiple_runs.sh`
`./multiple_runs.sh`
