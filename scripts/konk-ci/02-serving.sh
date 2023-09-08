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

set -eo pipefail
set -u

KNATIVE_VERSION=${KNATIVE_VERSION:-1.4.0}

wget -q https://github.com/knative/client/releases/download/knative-v${KNATIVE_VERSION}/kn-linux-amd64
mv kn-linux-amd64 kn && chmod +x kn
mv kn /usr/local/bin

n=0
set +e
until [ $n -ge 2 ]; do
  kubectl apply -f https://github.com/knative/serving/releases/download/knative-v${KNATIVE_VERSION}/serving-crds.yaml && break
  echo "Serving CRDs failed to install on first try"
  n=$[$n+1]
  sleep 5
done
set -e
kubectl wait --for=condition=Established --all crd

n=0
set +e
until [ $n -ge 2 ]; do
  kubectl apply -f https://github.com/knative/serving/releases/download/knative-v${KNATIVE_VERSION}/serving-core.yaml && break
  echo "Serving Core failed to install on first try"
  n=$[$n+1]
  sleep 5
done
set -e
kubectl wait pod --timeout=-1s --for=condition=Ready -l '!job-name' -n knative-serving

