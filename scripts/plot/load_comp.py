import sys
import os
sys.path.insert(0, '../../../sampler/sampler/')

from util import *
from sampler import *

rand_loads = []
sizes = list(range(100, 500, 20))
for size in sizes:
    rundf = pd.read_csv(f'../../../sampler/data/random/{size}_run.csv')
    memdf = pd.read_csv(f'../../../sampler/data/random/{size}_mem.csv')    
    invdf = pd.read_csv(f'../../../sampler/data/random/{size}_inv.csv')  
    loads = sum(get_function_loads(invdf, rundf, memdf, kind='mul'))
    rand_loads.append(loads)

harmonic_loads = []
mul_loads = []
sizes = list(range(100, 500, 20))
for size in sizes:
    rundf = pd.read_csv(f'../../../github/sampler/data/samples-10-1k/{size}_run.csv')
    memdf = pd.read_csv(f'../../../github/sampler/data/samples-10-1k/{size}_mem.csv')    
    invdf = pd.read_csv(f'../../../github/sampler/data/samples-10-1k/{size}_inv.csv')  
    loads = sum(get_function_loads(invdf, rundf, memdf, kind='harmonic'))
    loads = sum(get_function_loads(invdf, rundf, memdf, kind='mul'))
    mul_loads.append(loads)

fig= plt.figure(figsize=(15,5))
gs = fig.add_gridspec(1, 2, wspace=0.02)
ax1, ax2 = gs.subplots(sharey=True, sharex=True)

# Plotting in million scale
sns.barplot(ax=ax1, x=sizes[:11], y=(np.array(rand_loads[:11])/1e9), color='gray', lw=2, ec='k',)
sns.barplot(ax=ax2, x=sizes[:11], y=(np.array(mul_loads[:11])/1e9).round(), color='lightgray', lw=2, ec='k',)

plt.show()