#!/bin/bash

# Array of IP addresses of the target machines
target_ips=("pc753.emulab.net")

# Path to the Python script you want to run on the remote machines
python_script="imageDelete.py"

# Loop through the target IPs
for ip in "${target_ips[@]}"; do
    echo "Copying the cleaning script to $ip..."

    # Connect to the remote machine and start a tmux session
    ssh lfavento@$ip << EOF
        echo "Starting a tmux util..."
        tmux new-session -d -s util
        tmux send-keys -t util "cd tools/imageDelete" C-m
        tmux send-keys -t util "python3 $python_script" C-m
        tmux send-keys -t util "rm -f imageList.txt" C-m
        tmux attach -t util
EOF

    echo "Cleaning image script on $ip is complete."
done
