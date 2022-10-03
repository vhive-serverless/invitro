import sys
import os
sys.path.insert(0, '../../../sampler/sampler/')

from util import *
from sampler import *

from scipy.stats import wasserstein_distance as wd
from scipy.spatial.distance import jensenshannon as js

mem_df = load_data('../../../sampler/data/trace/mem.t')
wd_results = {'Uniform Random':[], 'Roll-Up Scaling':[], 'size':[]}
sizes = list(range(10, 1001, 10))
for size in tqdm(sizes):
    wd_random = wd(mem_df.AverageAllocatedMb_pct50,  
        pd.read_csv(f'../../../sampler/data/random-distance/{size}_mem.csv').AverageAllocatedMb_pct50)
    wd_sample = wd(mem_df.AverageAllocatedMb_pct50, 
        pd.read_csv(f'../../../github/sampler/data/samples-10-1k/{size}_mem.csv').AverageAllocatedMb_pct50)
    wd_results['Roll-Up Scaling'].append(wd_sample)    
    wd_results['Uniform Random'].append(wd_random)
    wd_results['size'].append(size)
wd_df = pd.DataFrame.from_dict(wd_results)
wd_df = pd.melt(wd_df.reset_index(), id_vars=['index','size'], value_vars=['Uniform Random', 'Roll-Up Scaling'], var_name='kind', value_name='distance')

## Plotting
sns.lineplot(data=wd_df, x='size', y='distance', hue='kind', marker='', palette=['r', 'b'], lw=2)
plt.show()  
