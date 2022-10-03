import sys
import os
import pandas as pd
from tqdm import tqdm
from glob import glob
import numpy as np
import seaborn as sns
import matplotlib.pyplot as plt

def percentile(n):
    def percentile_(x):
        return np.percentile(x, n)
    percentile_.__name__ = 'percentile_%s' % n
    return percentile_

p2 = pd.DataFrame()
dirs = ['rollup1','rollup2','rollup3','rollup4','rollup5','rollup6','rollup7','rollup8','rollup9','rollup10']
# dirs = [ '/Users/ustiugov/repos/loader2/results_loader/2w-hytrace-no-assertion-1/out', ]
for sample_size in range(10, 210, 10):
    for d in tqdm(dirs):
        f = glob(f"sweep/{d}/exec*sample-{sample_size}_*phase-2*.csv")
        if not f:
            continue
        else:
            f = f[0]

        df = pd.read_csv(f)
        sample_size = int(os.path.basename(f).split('sample-')[1].split('_phase')[0])
        df['sample_size'] = sample_size; threshold=40
        ## Aglin timestamps
        df.sort_values(by='timestamp', inplace=True)
        offset = df.timestamp.values[0]
        df['timestamp'] -= offset
        df['timestamp'] = pd.to_datetime(df['timestamp'], unit='us')

        df['latency'] = (df.response_time - df.actual_duration)/1e6
        # df['slowdown'] = df.response_time / df.actual_duration
        df['slowdown'] = df.response_time / df.requested_duration

        p2=p2.append(df)

p2.reset_index(drop=True, inplace=True)
print(p2.shape, p2.columns, set(p2.phase), len(set(p2.sample_size)))

p2_full = p2.copy()
p2 = p2[(p2['response_time'] > 0) & (p2['actual_duration'] > 0)] ## Filter out failure values


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
        sample_size = int(os.path.basename(f).split('sample-')[1].split('_phase')[0])
        df['sample_size'] = sample_size; threshold=40
        ## Aglin timestamps
        df.sort_values(by='timestamp', inplace=True)
        offset = df.timestamp.values[0]
        df['timestamp'] -= offset
        df['timestamp'] = pd.to_datetime(df['timestamp'], unit='us')

        df['latency'] = (df.response_time - df.actual_duration)/1e6
        # df['slowdown'] = df.response_time / df.actual_duration
        df['slowdown'] = df.response_time / df.requested_duration

    rand=rand.append(df)

rand.reset_index(drop=True, inplace=True)
rand_full = rand.copy()
rand = rand[(rand['response_time'] > 0) & (rand['actual_duration'] > 0)] ## Filter out failure values

ax=sns.lineplot(data=p2[p2['sample_size'] < 210], x='sample_size', y='slowdown', marker='^', label='Roll-Up', lw=2, alpha=1, color='b')
plt.show()  
