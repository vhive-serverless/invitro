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

source /etc/profile

# KWOK repository
KWOK_REPO=kubernetes-sigs/kwok
# Get latest
KWOK_LATEST_RELEASE=$(curl "https://api.github.com/repos/${KWOK_REPO}/releases/latest" | jq -r '.tag_name')

go install sigs.k8s.io/kwok/cmd/{kwok,kwokctl}@${KWOK_LATEST_RELEASE}

# KWOK repository
KWOK_REPO=kubernetes-sigs/kwok

# Get latest
KWOK_LATEST_RELEASE=$(curl "https://api.github.com/repos/${KWOK_REPO}/releases/latest" | jq -r '.tag_name')

kubectl apply -f "https://github.com/${KWOK_REPO}/releases/download/${KWOK_LATEST_RELEASE}/kwok.yaml"

kubectl apply -f "https://github.com/${KWOK_REPO}/releases/download/${KWOK_LATEST_RELEASE}/stage-fast.yaml"

kubectl apply -f "https://github.com/${KWOK_REPO}/releases/download/${KWOK_LATEST_RELEASE}/metrics-usage.yaml"

kubectl apply -f config/kwok_setup.yaml

export KWOK_NODE_NAME=kwok-fake-node
envsubst < config/kwok_fake_node.yaml | kubectl apply -f -

kubectl create namespace kwok-system
kubectl apply -f config/deploy_timer.yaml

kubectl patch deployment activator -n knative-serving -p '{"spec": {"template": {"spec": {"containers": [{"name": "activator", "image": "docker.io/wanghanchengchn/activator-ecd51ca5034883acbe737fde417a3d86@sha256:fc47373ba5794282c6a22518a7e78040067fd14dbb143c37f5065caff4622a89"}]}}}}'