#!/usr/bin/env bash

CAPACITY=$1
kubectl patch configmap config-autoscaler -n knative-serving -p '{"data":{"activator-capacity":'$CAPACITY'}}'