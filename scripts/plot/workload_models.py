import sys
import os
sys.path.insert(0, '../../../sampler/sampler/')

from util import *
from sampler import *

def index_run_mem(inv_sample, size, name, output):
    run_sample = pd.DataFrame()
    mem_sample = pd.DataFrame()
    for i, record in tqdm(inv_sample.iterrows(), total=len(inv_sample)):
        app = record.HashApp
        func = record.HashFunction

        memorydf = mem_index.loc[app]
        durationdf = run_index.loc[func]

        memorydf = memorydf.sample(n=1) if len(memorydf.shape)>1 else memorydf
        mem_sample = mem_sample.append(memorydf)

        durationdf = durationdf.sample(n=1) if len(durationdf.shape)>1 else durationdf
        run_sample = run_sample.append(durationdf)

    run_sample.reset_index(inplace=True)
    mem_sample.reset_index(inplace=True)

    if 'index' in list(run_sample.columns):
        run_sample.columns = list(map(lambda x: x.replace('index', 'HashFunction'), list(run_sample.columns)))
    if 'index' in list(mem_sample.columns):
        mem_sample.columns = list(map(lambda x: x.replace('index', 'HashApp'), list(mem_sample.columns)))

    # log.info(f"inv-func: {len(set(inv_sample.HashFunction))}, inv-app: {len(set(inv_sample.HashApp))}, inv/\\run: {len(set(inv_sample.HashFunction).intersection(run_sample.HashFunction))}, inv/\\mem: {len(set(inv_sample.HashApp).intersection(mem_sample.HashApp))}")

    save_data(inv_sample, f"{output}/{size}_inv_{name}.csv", csv=True)
    save_data(mem_sample, f"{output}/{size}_mem_{name}.csv", csv=True)
    save_data(run_sample, f"{output}/{size}_run_{name}.csv", csv=True)
    
    return 


## Trace
inv_df = load_data('../../../sampler/data/trace/inv.t') # Original (Hermes, Medes, Faascache)
# inv_df = load_data('../../../sampler/data/trace/inv_trigger.t') # With Triggers (Atoll)
mem_df = load_data('../../../sampler/data/trace/mem.t')
run_df = load_data('../../../sampler/data/trace/run.t')
inv_df = agg_horizontal(inv_df, 'total')
run_index = run_df.set_index('HashFunction')
mem_index = mem_df.set_index('HashApp')

## Our samples
inv_sample = pd.read_csv('../../../github/sampler/data/samples2/50_inv.csv')
mem_sample = pd.read_csv('../../../github/sampler/data/samples2/50_mem.csv')
run_sample = pd.read_csv('../../../github/sampler/data/samples2/50_run.csv')
inv_sample = agg_horizontal(inv_sample, 'total')

## Hermes
med_load = np.floor(np.mean(inv_df.total))
large_load = np.floor(med_load*.9)
small_load = np.floor(mde_load*.1/49)
hermes = pd.DataFrame()
hermes = hermes.append(inv_df[inv_df['total']==large_load].iloc[0, :])
# Random state: 1 and 4
hermes = hermes.append(inv_df[inv_df['total']==med_load].sample(n=49, replace=False, random_state=0))
hermes.to_csv('../../../sampler/data/distributions/hermes.csv', index=False)

## Medes
medes = inv_df.sample(n=50, replace=False, random_state=0).reset_index(drop=True)
medes_mem = np.array([17, 32, 26, 48, 32, 22, 22, 66, 90, 88]*5).astype(int)
medes_run = np.array([150, 250, 1200, 2000, 500, 400, 400, 1000, 1000, 3000]*5).astype(int)
medes = agg_horizontal(medes, 'total')
medes['total'] = medes.total*5 # Scale 5x
medes_mem_df = medes.drop(medes.index)
medes_mem_df['HashApp'] = medes.HashApp
medes_mem_df['HashOwner'] = medes.HashOwner
medes_run_df = run_df.drop(run_df.index)
medes_run_df[['HashFunction', 'HashOwner', 'HashApp']] = medes[['HashFunction', 'HashOwner', 'HashApp']]
for i in range(10):
    for col in medes_mem_df.columns[2:]:
        medes_mem_df.at[i, col] = int(medes_mem[i])
    for col in medes_run_df.columns[3:]:
        medes_run_df.at[i, col] = int(medes_run[i])
medes_run_df['Count'] = 0 
medes_mem_df['SampleCount'] = 0
medes.to_csv('../../../sampler/data/distributions/medes.csv', index=False)

## FaasCache
faascache = inv_df.sample(n=50, replace=False, random_state=1).reset_index(drop=True)
faascache_df = faascache.iloc[:, 3:]
faascache_df = faascache_df.T.groupby(faascache_df.T.reset_index(drop=True).index//60).sum().T
faascache_df = faascache.iloc[:, :3].join(faascache_df)
faascache = agg_horizontal(faascache, 'total')
faascache.to_csv('../../../sampler/data/distributions/faascache.csv', index=False)
index_run_mem(faascache, 200, 'faascache', '../../../sampler/data/workloads')

## Atoll
atoll = pd.DataFrame()
for trigger in ['event', 'http', 'orchestration', 'others', 'queue', 'storage', 'timer']:
    df = inv_df[inv_df.Trigger==trigger].sort_values('total', ascending=False).iloc[:8, :]
    atoll = atoll.append(df)
atoll = atoll.iloc[:50, :].reset_index(drop=True)
atoll.to_csv('../../../sampler/data/distributions/atoll.csv', index=False)

## Plotting
ax=sns.ecdfplot(data=inv_df.total, log_scale=True, c="r", label='Azure Trace', alpha=0.8, lw=3)
sns.ecdfplot(ax=ax,data=inv_sample.total, log_scale=True, c="b", label='In Vitro (Ours)', alpha=0.8, lw=2)
sns.ecdfplot(ax=ax,data=faascache.total, log_scale=True, c="k", label='Random (FaasCache)', alpha=0.8, lw=1.5, ls='--')
sns.ecdfplot(ax=ax,data=medes.total, log_scale=True, c="orange", label='Random+Scaling (Medes)', alpha=0.8, lw=2, ls='-.')
sns.ecdfplot(ax=ax,data=hermes.total, log_scale=True, c="g", label='Load-Based (Hermes)', alpha=0.8, lw=3, ls=':')
sns.ecdfplot(ax=ax,data=atoll.total, log_scale=True, c="c", label='Event-Based (Atoll*)', alpha=0.8, lw=3, ls='dotted')

plt.show()  
