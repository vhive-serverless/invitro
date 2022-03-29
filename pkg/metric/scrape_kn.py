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
        "coldstart_count": get_promql_query('sum(activator_request_count)'),

        "autoscaler_stable_queue": get_promql_query('avg(autoscaler_stable_request_concurrency)'),
        "autoscaler_panic_queue": get_promql_query('avg(autoscaler_panic_request_concurrency)'),
        "activator_queue": get_promql_query('avg(activator_request_concurrency)'),
    }

    for label, query in kn_statua.items():

        while True:
            try:
                measure = os.popen(query()).read().strip()
                if 'error' not in measure:
                    break
            except:
                pass

        if label.startswith('a'):
            measure = float(measure) if measure else 0.0
        else:
            measure = int(measure) if measure else 0
            
        kn_statua[label] = measure
    
    print(json.dumps(kn_statua))

    
