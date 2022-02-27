import os
import json

def get_promql_query(query):
    def promql_query():
        return "tools/bin/promql --no-headers --host 'http://localhost:9090' '" + query + "' | awk '{print $1}'"
    return promql_query

if __name__ == "__main__":
    kn_statua = {
        # Desired counts set by autoscalers.
        "desired_pods": get_promql_query('sum(autoscaler_desired_pods)'), 
        # Creating containers.
        "unready_pods": get_promql_query('sum(autoscaler_not_ready_pods)'),
        # Scheduling + image pulling.
        "pending_pods": get_promql_query('sum(autoscaler_pending_pods)'),
        # Number of pods autoscalers requested from Kubernetes.
        "requested_pods": get_promql_query('sum(autoscaler_requested_pods)'),
        "running_pods": get_promql_query('sum(autoscaler_actual_pods)'),

        "autoscaler_stable_queue": get_promql_query('avg(autoscaler_stable_request_concurrency)'),
        "autoscaler_pandic_queue": get_promql_query('avg(autoscaler_panic_request_concurrency)'),
        "activator_queue": get_promql_query('avg(activator_request_concurrency)'),
        "coldstart_count": get_promql_query('sum(activator_request_count)'),
    }

    for label, query in kn_statua.items():
        kn_statua[label] = os.popen(query()).read().strip()
    
    print(json.dumps(kn_statua))

    
