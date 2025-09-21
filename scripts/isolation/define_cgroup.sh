#!/usr/bin/env bash

#
# MIT License
#
# Copyright (c) 2023 EASL and the vHive community
#
# Permission is hereby granted, free of charge, to any person obtaining a copy
# of this software and associated documentation files (the "Software"), to deal
# in the Software without restriction, including without limitation the rights
# to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
# copies of the Software, and to permit persons to whom the Software is
# furnished to do so, subject to the following conditions:
#
# The above copyright notice and this permission notice shall be included in all
# copies or substantial portions of the Software.
#
# THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
# IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
# FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
# AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
# LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
# OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
# SOFTWARE.
#

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