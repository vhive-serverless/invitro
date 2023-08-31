import json
import os, sys 

for config_name in ['config_client_hived_elastic_real', 'config_client_batch_real']: 
    # Read the JSON file
    with open(f'cmd/real_configs/{config_name}.json', 'r') as f:
        data = json.load(f)

    # Change the value for the key "TracePath"


    # Save the modified JSON back to the file
    for load in [0.2, 0.25, 0.3, 0.35, 0.4, 0.45, 0.5, 0.6, 0.7, 0.8, 0.9]: 
        data['TracePath'] = f"data/real-multi-func-gputraces/jobload-{load}"
        data['OutputPathPrefix'] = f"data/out/real-multi-experiment-jobload-{load}"
        data['EnableMetricsScrapping'] = True 
        data['MetricScrapingPeriodSeconds'] = 5
        # import pdb; pdb.set_trace() 
        with open(f'cmd/real_configs/{config_name}-{load}.json', 'w') as f:
            json.dump(data, f, indent=4)
