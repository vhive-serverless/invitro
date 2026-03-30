#  MIT License
#
#  Copyright (c) 2023 EASL and the vHive community
#
#  Permission is hereby granted, free of charge, to any person obtaining a copy
#  of this software and associated documentation files (the "Software"), to deal
#  in the Software without restriction, including without limitation the rights
#  to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
#  copies of the Software, and to permit persons to whom the Software is
#  furnished to do so, subject to the following conditions:
#
#  The above copyright notice and this permission notice shall be included in all
#  copies or substantial portions of the Software.
#
#  THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
#  IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
#  FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
#  AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
#  LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
#  OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
#  SOFTWARE.

import json
import os

prometheus_ip = os.popen("kubectl get svc -n monitoring | grep prometheus-kube-prometheus-prometheus | awk '{print $3}'").read().strip().split('\n')[0]

def get_promql_query(query):
    def promql_query():
        return "tools/bin/promql --no-headers --host 'http://" + prometheus_ip + ":9090' '" + query + "' | grep . | awk '{print $1\" \"$2}'"
    return promql_query


def parse_metric_map(query, caster):
    metrics = {}
    output = os.popen(get_promql_query(query)()).read()

    for raw_line in output.splitlines():
        line = raw_line.strip()
        if not line:
            continue

        parts = line.split()
        if len(parts) < 2:
            continue

        try:
            metrics[parts[0]] = caster(parts[1])
        except ValueError:
            continue

    return metrics


def to_int(value):
    return int(float(value))

if __name__ == "__main__":
    query_desired_pods = 'max(autoscaler_desired_pods) by(configuration_name)'
    query_running_pods = 'max(autoscaler_actual_pods) by(configuration_name)'
    query_unready_pods = 'max(autoscaler_not_ready_pods) by(configuration_name)'
    query_pending_pods = 'max(autoscaler_pending_pods) by(configuration_name)'
    query_terminating_pods = 'max(autoscaler_terminating_pods) by(configuration_name)'
    query_activator_queue = 'sum(activator_request_concurrency) by(configuration_name)'

    desired_pods_count = parse_metric_map(query_desired_pods, to_int)
    running_pods_count = parse_metric_map(query_running_pods, to_int)
    unready_pods_count = parse_metric_map(query_unready_pods, to_int)
    pending_pods_count = parse_metric_map(query_pending_pods, to_int)
    terminating_pods_count = parse_metric_map(query_terminating_pods, to_int)
    queue_size = parse_metric_map(query_activator_queue, float)

    results = []
    for func in desired_pods_count.keys():
        results.append({
            'function': func,
            'desired_pods': desired_pods_count[func],
            'running_pods': running_pods_count.get(func, 0),
            'unready_pods': unready_pods_count.get(func, 0),
            'pending_pods': pending_pods_count.get(func, 0),
            'terminating_pods': terminating_pods_count.get(func, 0),
            'activator_queue': queue_size.get(func, 0)
        })

    print(json.dumps(results))
