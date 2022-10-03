import sys
import os
sys.path.insert(0, '../../../sampler/sampler/')

from util import *
from sampler import *

p3 = pd.DataFrame()
for kind in ['sampler', 'random']:
    p3_exf = glob(f"../../data/converge/{kind}/*/exec*phase-2_*.csv")[:101]
    if not p3_exf: continue

    p3_ex_df = pd.DataFrame()
    for f in p3_exf:
        df = pd.read_csv(f)
        df.sort_values(by='timestamp', inplace=True)
        sample_size = int(os.path.basename(f).split('sample-')[1].split('_phase')[0])
        df['kind'] = kind
        if kind == 'sampler':
            df['label'] = 'Ours'
        elif kind == 'random':
            df['label'] = 'Uniform Random'
        p3_ex_df = p3_ex_df.append(df)
    p3_ex_df.sort_values(by='timestamp', inplace=True)
    p3_ex_df['timestamp'] = pd.to_datetime(p3_ex_df['timestamp'], unit='us')
    ##* In seconds.
    p3_ex_df['latency'] = (p3_ex_df.response_time - p3_ex_df.actual_duration)/1e6
    p3_ex_df['slowdown'] = p3_ex_df.response_time / p3_ex_df.requested_duration

    p3 = p3.append(p3_ex_df)
    
p3 = p3.reset_index(drop=True)

## Plotting
ax=sns.ecdfplot(data=p3_p50[p3_p50.kind=='sampler'], x='slowdown', log_scale=True, lw=2, palette='bright', legend=True, label='In Vitro Sampling', c='b')
sns.ecdfplot(ax=ax, data=p3_p50[p3_p50.kind=='random'], x='slowdown', log_scale=True, lw=2, palette='bright', legend=True, label='Uniform Random', c='r')
plt.show()  
