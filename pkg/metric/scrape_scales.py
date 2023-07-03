import json
import os

prometheus_ip = os.popen("kubectl get svc -n monitoring | grep prometheus-kube-prometheus-prometheus | awk '{print $3}'").read().strip().split('\n')[0]

def get_promql_query(query):
    def promql_query():
        return "tools/bin/promql --no-headers --host 'http://" + prometheus_ip + ":9090' '" + query + "' | grep . | awk '{print $1\" \"$2}'"
    return promql_query

if __name__ == "__main__":
    query_desired_pods = 'max(autoscaler_desired_pods) by(configuration_name)'
    query_running_pods = 'max(autoscaler_actual_pods) by(configuration_name)'
    query_unready_pods = 'max(autoscaler_not_ready_pods) by(configuration_name)'
    query_pending_pods = 'max(autoscaler_pending_pods) by(configuration_name)'
    query_terminating_pods = 'max(autoscaler_terminating_pods) by(configuration_name)'
    query_activator_queue = 'sum(activator_request_concurrency) by(configuration_name)'

    desired_pods_count = {x.split()[0]: int(x.split()[1]) for x in os.popen(get_promql_query(query_desired_pods)()).read().strip().split('\n')}
    running_pods_count = {x.split()[0]: int(x.split()[1]) for x in os.popen(get_promql_query(query_running_pods)()).read().strip().split('\n')}
    unready_pods_count = {x.split()[0]: int(x.split()[1]) for x in os.popen(get_promql_query(query_unready_pods)()).read().strip().split('\n')}
    pending_pods_count = {x.split()[0]: int(x.split()[1]) for x in os.popen(get_promql_query(query_pending_pods)()).read().strip().split('\n')}
    terminating_pods_count = {x.split()[0]: int(x.split()[1]) for x in os.popen(get_promql_query(query_terminating_pods)()).read().strip().split('\n')}
    queue_size = {x.split()[0]: float(x.split()[1]) for x in os.popen(get_promql_query(query_activator_queue)()).read().strip().split('\n')}

    results = []
    for func in desired_pods_count.keys():
        results.append({
            'function': func,
            'desired_pods': desired_pods_count[func],
            'running_pods': running_pods_count[func],
            'unready_pods': unready_pods_count[func],
            'pending_pods': pending_pods_count[func],
            'terminating_pods': terminating_pods_count[func],
            'activator_queue': queue_size.get(func, 0)
        })

    print(json.dumps(results))
