import os
import json

def get_promql_query(query):
    def promql_query():
        return "tools/bin/promql --no-headers --host 'http://localhost:9090' '" + query + "' | awk '{print $1}'"
    return promql_query

if __name__ == "__main__":
    kn_status = {
        # Desired counts set by autoscalers.
        "desired_pods": get_promql_query('sum(autoscaler_desired_pods)'), 
        # Creating containers.
        "unready_pods": get_promql_query('sum(autoscaler_not_ready_pods)'),
        # Scheduling + image pulling.
        "pending_pods": get_promql_query('sum(autoscaler_pending_pods)'),
        # Number of pods autoscalers requested from Kubernetes.
        "requested_pods": get_promql_query('sum(autoscaler_requested_pods)'),
        "running_pods": get_promql_query('sum(autoscaler_actual_pods)'),
        "activator_request_count": get_promql_query('sum(activator_request_count)'),

        "autoscaler_stable_queue": get_promql_query('avg(autoscaler_stable_request_concurrency)'),
        "autoscaler_panic_queue": get_promql_query('avg(autoscaler_panic_request_concurrency)'),
        "activator_queue": get_promql_query('avg(activator_request_concurrency)'),
        
        # The p99 latency of single scheduling round (algorithm+binding) over a time window of 30s.
        "scheduling_p99": get_promql_query(
            'histogram_quantile(0.99, sum by (le) (rate(scheduler_e2e_scheduling_duration_seconds_bucket{job="kube-scheduler"}[30s])))'
        ), 
        "scheduling_p50": get_promql_query(
            'histogram_quantile(0.50, sum by (le) (rate(scheduler_e2e_scheduling_duration_seconds_bucket{job="kube-scheduler"}[30s])))'
        ),  

        # The p99 latency of E2E pod placement (potentially multiple scheduling rounds) over a time window of 30s.
        "e2e_placement_p99": get_promql_query(
            'histogram_quantile(0.99, sum by (le) (rate(scheduler_pod_scheduling_duration_seconds_bucket{job="kube-scheduler"}[30s])))'
        ),
        "e2e_placement_p50": get_promql_query(
            'histogram_quantile(0.50, sum by (le) (rate(scheduler_pod_scheduling_duration_seconds_bucket{job="kube-scheduler"}[30s])))'
        ), 
    }

    for label, query in kn_status.items():

        while True:
            try:
                measure = os.popen(query()).read().strip()
                if 'error' not in measure:
                    break
            except:
                pass

        if label.endswith('queue'):
            measure = float(measure) if measure else -99
        elif 'p50' in label or 'p99' in label:
            if measure == 'NaN': 
                # Not available.
                measure = -99
            else: 
                measure = float(measure) if measure else -99
        else:
            measure = int(measure) if measure else -99
            
        kn_status[label] = measure
    
    print(json.dumps(kn_status))

    
