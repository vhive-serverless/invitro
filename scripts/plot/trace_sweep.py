import sys
import os
import pandas as pd
from tqdm import tqdm
from glob import glob
import numpy as np
import seaborn as sns

# Put sampler in the following dir.
sys.path.insert(0, '../../../sampler/sampler/')
from util import *
from sampler import *

p2 = pd.DataFrame()
dirs = ['rollup1','rollup2','rollup3','rollup4','rollup5','rollup6','rollup7','rollup8','rollup9','rollup10']
for sample_size in range(10, 1000, 10):
    for d in tqdm(dirs):
        f = glob(f"../../data/sweep/{d}/exec*sample-{sample_size}_*phase-2*.csv")
        if not f: 
            continue
        else:
            f = f[0]

        df = pd.read_csv(f)
        # df.sort_values(by='timestamp', inplace=True)
        sample_size = int(os.path.basename(f).split('sample-')[1].split('_phase')[0])
        df['sample_size'] = sample_size; threshold=40
        ## Aglin timestamps
        for i, record in df.iterrows(): 
            if record.sample_size<=threshold:df.at[i,'response_time']=min(record.response_time,threshold*record.requested_duration)
        offset = df.timestamp.values[0]
        df['timestamp'] -= offset    
        df.sort_values(by='timestamp', inplace=True)
        
        df['timestamp'] = pd.to_datetime(df['timestamp'], unit='us')
        df['latency'] = (df.response_time - df.actual_duration)/1e6
        # df['slowdown'] = df.response_time / df.actual_duration
        df['slowdown'] = df.response_time / df.requested_duration

        p2=p2.append(df)

p2.reset_index(drop=True, inplace=True)
print(p2.shape, p2.columns, set(p2.phase), len(set(p2.sample_size)))
p2_full = p2.copy()
p2 = p2[(p2['response_time'] > 0) & (p2['actual_duration'] > 0)] ## Filter out failure values
p2_avg = p2.groupby(['sample_size']).agg({'latency': np.mean, 'slowdown': np.mean}).reset_index()
p2_p50 = p2.groupby(['sample_size']).agg({'latency': np.median, 'slowdown': np.median}).reset_index()
p2_p99 = p2.groupby(['sample_size']).agg({'latency': percentile(99), 'slowdown': percentile(99)}).reset_index()
p2_cpu = p2.groupby(['sample_size']).agg({'worker_cpu_active_avg': np.mean}).reset_index()
p2_mem = p2.groupby(['sample_size']).agg({'worker_mem': np.mean}).reset_index()
print(p2.shape)


rand = pd.DataFrame()
dirs = ['random1','random2','random3','random4','random5','random6','random7','random8','random9','random10']
for sample_size in range(10, 1000, 10):
    for d in tqdm(dirs):
        f = glob(f"../../data/sweep/{d}/exec*sample-{sample_size}_*phase-2*.csv")
        if not f: 
            continue
        else:
            f = f[0]
        df = pd.read_csv(f)
        df.sort_values(by='timestamp', inplace=True)
        sample_size = int(os.path.basename(f).split('sample-')[1].split('_phase')[0])
        df['sample_size'] = sample_size; threshold=40
        ## Aglin timestamps
        for i, record in df.iterrows(): 
            if record.sample_size<=threshold:df.at[i,'response_time']=min(record.response_time,threshold*record.requested_duration)
        offset = df.timestamp.values[0]
        df.sort_values(by='timestamp', inplace=True)
        
        df['timestamp'] -= offset    
        df.sort_values(by='timestamp', inplace=True)
        df['timestamp'] = pd.to_datetime(df['timestamp'], unit='us')
        df['latency'] = (df.response_time - df.actual_duration)/1e6
        # df['slowdown'] = df.response_time / df.actual_duration
        df['slowdown'] = df.response_time / df.requested_duration
    
    rand=rand.append(df)

rand.reset_index(drop=True, inplace=True)
rand_full = rand.copy()
rand = rand[(rand['response_time'] > 0) & (rand['actual_duration'] > 0)] ## Filter out failure values
print(rand.shape)

rand_avg = rand.groupby(['sample_size']).agg({'latency': np.mean, 'slowdown': np.mean}).reset_index()
rand_p50 = rand.groupby(['sample_size']).agg({'latency': np.median, 'slowdown': np.median}).reset_index()
rand_p99 = rand.groupby(['sample_size']).agg({'latency': percentile(99), 'slowdown': percentile(99)}).reset_index()
rand_cpu = rand.groupby(['sample_size']).agg({'worker_cpu_active_avg': np.mean}).reset_index()
rand_mem = rand.groupby(['sample_size']).agg({'worker_mem': np.mean}).reset_index()

ax=sns.lineplot(data=p2[p2['sample_size'] < 210], x='sample_size', y='slowdown', marker='^', label='Roll-Up', lw=2, alpha=1, color='b', ci=None)
sns.lineplot(ax=ax, data=rand[rand['sample_size'] < 210], x='sample_size', y='slowdown',label='Unif. Rand', marker='v', lw=2, alpha=1, color='r', ci=None)
sns.show()