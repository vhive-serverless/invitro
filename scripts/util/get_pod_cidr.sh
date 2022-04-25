#!/usr/bin/env bash

kubectl get nodes -o jsonpath='{.items[*].spec.podCIDR}'