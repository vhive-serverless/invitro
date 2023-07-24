from util import *
import os
import pandas as pd
import numpy as np
import string
import random
import copy 

if __name__ == '__main__': 
    output_path = '/users/gaow0007/loader-gpt/data/real-gputraces/example/'
    # df = pd.read_csv(f'{output_path}/invocations.csv')
    ref_df = pd.read_csv(f'{output_path}/reference.csv')
    
    keys = ['HashOwner', 'HashApp', 'HashFunction', 'Trigger']
    values = ['c455703077a17a9b8d0fc655d939fcc6d24d819fa9a1066b74f710c35a43cbc8',
              '68baea05aa0c3619b6feb78c80a07e27e4e68f921d714b8125f916c3b3370bf2',
              'c13acdc7567b225971cef2416a3a2b03c8a4d8d154df48afe75834e2f5c59ddf',
              'queue']
    submission_time_info = ref_df.submission_time.tolist() 
    num_gpu_info = ref_df.num_gpus.tolist() 
    # import pdb; pdb.set_trace() 
    for i in range(360): 
        keys.append(i+1)
        num_jobs = sum([time_info == i for time_info in submission_time_info])
        values.append(num_jobs)
    with open(f'{output_path}/invocations.csv', 'w') as f: 
        for idx, key in enumerate(keys): 
            f.write(f'{key}')
            if idx < len(keys) - 1: 
                f.write(',')
            else: 
                f.write('\n')
        
        for idx, value in enumerate(values): 
            f.write(f'{value}')
            if idx < len(values) - 1: 
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
    
    for row in range(len(df)): 
        for i in range(1, max_requests + 1): 
            insert_key = str(i)
            
            iteration_df.iloc[row, start_index - 1 + i] = np.random.randint(10, 100) // 10 * 100
            # batch_df.iloc[row, start_index - 1+ i] = np.random.choice([32 * k for k in [1, 2, 4, 6, 8, 10, 12, 16, 20, 24, 32]])
            # batch_df.iloc[row, start_index - 1+ i] = np.random.choice([32 * k for k in [1, 2, 4, 8, 12, 16]], p=[0.4, 0.2, 0.15, 0.15, 0.05, 0.05])
            batch_df.iloc[row, start_index - 1+ i] = num_gpu_info[i-1] * 32
            # np.random.choice([32 * k for k in [1, 2, 4, 8]], p=[0.4, 0.3, 0.2, 0.1])
            # import pdb; pdb.set_trace() 
            print('process {}'.format(i))
    save_data(iteration_df, f'{output_path}/iterations.csv')
    save_data(batch_df, f'{output_path}/batch.csv')