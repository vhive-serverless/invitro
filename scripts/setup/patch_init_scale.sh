#!/usr/bin/env bash

kubectl patch configmap config-autoscaler -n knative-serving -p '{"data":{"initial-scale":"0","allow-zero-initial-scale":"true"}}'