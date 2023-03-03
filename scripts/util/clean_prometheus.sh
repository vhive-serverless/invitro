#!/usr/bin/env bash

if [ `kubectl get statefulset -A | grep prometheus-prometheus-kube-prometheus-prometheus | wc -l` -ne "0" ]
then
	kubectl rollout restart statefulset prometheus-prometheus-kube-prometheus-prometheus -n monitoring
fi
