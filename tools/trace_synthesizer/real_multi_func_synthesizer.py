from util import *
import os
import pandas as pd
import numpy as np
import string
import random
import copy 

import random
import numpy as np
seed=42
random.seed(seed)
np.random.seed(seed)

if __name__ == '__main__': 
    reference_path = '/users/gaow0007/loader-gpt/data/real-multi-func-gputraces/example'
    # df = pd.read_csv(f'{output_path}/invocations.csv')
    
    
    
    for scale in [0.2, 0.25, 0.3, 0.35, 0.4, 0.45, 0.5, 0.6, 0.7, 0.8, 0.9]: 
        keys = ['HashOwner', 'HashApp', 'HashFunction', 'Trigger']
        func_invocations = [            
            ['a4123','18ege','gpt2-base','queue'], 
            ['b4264','83hke','gpt2-large','queue'],
            ['c4575','38bae','llama-7b','queue'],
            #   ('d4557','48bae','gpt2-base','queue'),
            #   ('e4557','58bae','gpt2-base','queue'),
              ]
        for func_idx, func_info in enumerate(func_invocations): 
            ref_df = pd.read_csv(f'{reference_path}/reference_{func_idx}.csv')
            submission_time_info = ref_df.submission_time.tolist() 
            num_gpu_info = ref_df.num_gpus.tolist() 
            for i in range(360): 
                if func_idx == 0: 
                    keys.append(i+1)
                num_jobs = sum([time_info == i for time_info in submission_time_info])
                func_invocations[func_idx].append(int(num_jobs * scale))
        
        # import pdb; pdb.set_trace() 
        output_path =  f'/users/gaow0007/loader-gpt/data/real-multi-func-gputraces/jobload-{scale}'
        if not os.path.exists(output_path): 
            os.makedirs(output_path)
            
        with open(f'{output_path}/invocations.csv', 'w') as f: 
            for idx, key in enumerate(keys): 
                f.write(f'{key}')
                if idx < len(keys) - 1: 
                    f.write(',')
                else: 
                    f.write('\n')
            for invocations in func_invocations:
                for idx, value in enumerate(invocations): 
                    f.write(f'{value}')
                    if idx < len(invocations) - 1: 
                        f.write(',')
                    else: 
                        f.write('\n')
        print(f'{output_path}/invocations.csv')
        
        
        df = pd.read_csv(f'{output_path}/invocations.csv')
        
        start_index = 4
        max_length = 0 
        for row in range(len(df)):  
            total_requests = df.iloc[row, start_index:].sum()
            max_requests = max(total_requests, max_length)
        
        # import pdb; pdb.set_trace() 
        for i in range(1, max_requests + 1): 
            insert_key = str(i)
            if insert_key not in df: 
                df.insert(loc=len(df.columns), column=insert_key, value=0)
        
        batch_df = copy.deepcopy(df)
        iteration_df = copy.deepcopy(df)
        deadline_df = copy.deepcopy(df)
        for row in range(len(df)): 
            for i in range(1, max_requests + 1): 
                insert_key = str(i)
                iteration = np.random.randint(10, 100) // 10 * 200
                iteration_df.iloc[row, start_index - 1 + i] = iteration
                # batch_df.iloc[row, start_index - 1+ i] = np.random.choice([32 * k for k in [1, 2, 4, 6, 8, 10, 12, 16, 20, 24, 32]])
                # batch_df.iloc[row, start_index - 1+ i] = np.random.choice([32 * k for k in [1, 2, 4, 8, 12, 16]], p=[0.4, 0.2, 0.15, 0.15, 0.05, 0.05])
                batch_df.iloc[row, start_index - 1+ i] = num_gpu_info[i-1] * 32
                # ddl_ratio = np.random.choice([0.5 + 0.1 * k for k in range(11)])
                ddl_ratio = np.random.choice([0.5, 0.75, 1.0, 1.25, 1.5])
                deadline_df.iloc[row, start_index - 1 + i] = int(ddl_ratio * iteration * 100)
                # np.random.choice([32 * k for k in [1, 2, 4, 8]], p=[0.4, 0.3, 0.2, 0.1])
                # import pdb; pdb.set_trace() 
                print('process {}'.format(i))
        save_data(iteration_df, f'{output_path}/iterations.csv')
        save_data(batch_df, f'{output_path}/batch.csv')
        save_data(deadline_df, f'{output_path}/deadline.csv')
        
        # duration 
        duration_keys = 'HashOwner,HashApp,HashFunction,Average,Count,Minimum,Maximum,percentile_Average_0,percentile_Average_1,percentile_Average_25,percentile_Average_50,percentile_Average_75,percentile_Average_99,percentile_Average_100'
        duration_values = '100.0,57523.0,99.0,101.0,99.0,99.0,99.0,100.0,101.0,101.0,101.0'
        memory_keys = 'HashOwner,HashApp,HashFunction,SampleCount,AverageAllocatedMb,AverageAllocatedMb_pct1,AverageAllocatedMb_pct5,AverageAllocatedMb_pct25,AverageAllocatedMb_pct50,AverageAllocatedMb_pct75,AverageAllocatedMb_pct95,AverageAllocatedMb_pct99,AverageAllocatedMb_pct100'
        memory_values = '19342.0,120.0,100.0,102.0,114.0,123.0,127.0,136.0,143.0,152.0'
        with open(f'{output_path}/durations.csv', 'w') as f: 
            f.write(duration_keys + '\n')
            for innvocation in func_invocations: 
                for invoc in innvocation[:3]: 
                    f.write(f'{invoc},')
                f.write(f'{duration_values}\n')
        
        
        with open(f'{output_path}/memory.csv', 'w') as f: 
            f.write(memory_keys + '\n')
            for innvocation in func_invocations: 
                for invoc in innvocation[:3]: 
                    f.write(f'{invoc},')
                f.write(f'{memory_values}\n')
        