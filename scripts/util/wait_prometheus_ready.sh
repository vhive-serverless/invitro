#!/usr/bin/env bash

set -euo pipefail

namespace=${PROMETHEUS_NAMESPACE:-monitoring}
statefulset=${PROMETHEUS_STATEFULSET:-prometheus-prometheus-kube-prometheus-prometheus}
service=${PROMETHEUS_SERVICE:-prometheus-kube-prometheus-prometheus}
port=${PROMETHEUS_PORT:-9090}
timeout=${PROMETHEUS_READY_TIMEOUT_SECONDS:-120}
poll=${PROMETHEUS_READY_POLL_SECONDS:-2}

[[ "$timeout" =~ ^[0-9]+$ && "$timeout" -gt 0 ]] || {
    echo "PROMETHEUS_READY_TIMEOUT_SECONDS must be a positive integer" >&2
    exit 2
}
[[ "$poll" =~ ^[0-9]+$ && "$poll" -gt 0 ]] || {
    echo "PROMETHEUS_READY_POLL_SECONDS must be a positive integer" >&2
    exit 2
}

kubectl rollout status "statefulset/$statefulset" -n "$namespace" --timeout="${timeout}s"

deadline=$((SECONDS + timeout))
last_error="Prometheus readiness checks did not pass"
while ((SECONDS < deadline)); do
    cluster_ip=$(kubectl get service "$service" -n "$namespace" -o jsonpath='{.spec.clusterIP}' 2>/dev/null || true)
    endpoints=$(kubectl get endpoints "$service" -n "$namespace" -o jsonpath='{.subsets[*].addresses[*].ip}' 2>/dev/null || true)
    if [[ -z "$cluster_ip" || "$cluster_ip" == "None" ]]; then
        last_error="service $service has no ClusterIP"
    elif [[ -z "$endpoints" ]]; then
        last_error="service $service has no ready endpoints"
    elif ! curl -fsS --max-time "$poll" "http://${cluster_ip}:${port}/-/ready" >/dev/null 2>&1; then
        last_error="Prometheus /-/ready is not successful"
    elif ! curl -fsS --max-time "$poll" --get --data-urlencode 'query=up' \
        "http://${cluster_ip}:${port}/api/v1/query" 2>/dev/null |
        jq -e '.status == "success" and (.data.result | length) > 0' >/dev/null; then
        last_error="Prometheus up query returned no successful nonempty result"
    else
        echo "Prometheus is ready: service=$service cluster_ip=$cluster_ip endpoints=$endpoints"
        exit 0
    fi
    sleep "$poll"
done

echo "$last_error (timeout=${timeout}s)" >&2
exit 1
