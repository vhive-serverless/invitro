#!/usr/bin/env bash

#* Result line i: <CPU of node i>%,<Memory of node i>%
kubectl top nodes | tail -n +2 | awk '{print $3$5}'