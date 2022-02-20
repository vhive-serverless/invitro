#!/usr/bin/env bash

#* Results in CSV lines (line i: "<CPU of node i>","<Memory of node i>")
kubectl get --raw "/apis/metrics.k8s.io/v1beta1/nodes" | jq -r '.items[].usage| [.cpu,.memory] | @csv'