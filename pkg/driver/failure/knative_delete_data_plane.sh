#!/bin/bash

# activator
kubectl delete pod $(kubectl get pods -n knative-serving -o name | cut -c 5- | grep activator | tail -n 1) -n knative-serving &

# cluster-local-gateway
kubectl delete pod $(kubectl get pods -n istio-system -o name | grep cluster-local-gateway | cut -c 5- | tail -n 1) -n istio-system &

# istio-ingressgateway
kubectl delete pod $(kubectl get pods -n istio-system -o name | grep istio-ingressgateway | cut -c 5- | tail -n 1) -n istio-system &

# istiod
kubectl delete pod $(kubectl get pods -n istio-system -o name | grep istiod | cut -c 5- | tail -n 1) -n istio-system &