#!/usr/bin/env bash

CURRENT_USER=$SUDO_USER

sudo apt-get install cgroup-tools cgroupfs-mount libcgroup1 -y

# If it's not already mounted, mount the cpuset 
if [[ ! $(mount | grep cpuset) ]] ; then
    mount -t cgroup -ocpuset cpuset /sys/fs/cgroup/cpuset
fi

# Create a group with the $CURRENT_USER who is able to write the task file.
cgcreate -a $CURRENT_USER -t $CURRENT_USER -g cpuset,memory:loader-cg

# Assign a full socket (8 cores) to it.
echo "8-15" > /sys/fs/cgroup/cpuset/loader-cg/cpuset.cpus
echo "0" > /sys/fs/cgroup/cpuset/loader-cg/cpuset.mems

# Specify memory limit (20GiB).
echo "21470000000" > /sys/fs/cgroup/memory/loader-cg/memory.limit_in_bytes

echo 'CGroup loader-cg created'