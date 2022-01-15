#!/usr/bin/env bash
kubectl patch configmap config-autoscaler -n knative-serving \
    -p '{"data":{"initial-scale":"1","enable-scale-to-zero":"true","max-scale-up-rate": "1000.0"}}'