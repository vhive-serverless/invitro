from util import *
import os
import pandas as pd
import numpy as np
import string
import random
import copy 

if __name__ == '__main__': 
    output_path = '/users/eth-easl/loader/data/gpttraces/example/'
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
            
            iteration_df.iloc[row, start_index - 1 + i] = np.random.randint(10, 100) // 10 * 10
            batch_df.iloc[row, start_index - 1+ i] = np.random.choice([32 * k for k in [1, 2, 4, 6, 8, 10, 12, 16, 20, 24, 32]])
            # import pdb; pdb.set_trace() 
            print('process {}'.format(i))
    save_data(iteration_df, f'{output_path}/iterations.csv')
    save_data(batch_df, f'{output_path}/batch.csv')