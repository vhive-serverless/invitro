import os, sys
import matplotlib.pyplot as plt 
import seaborn
import matplotlib
import numpy as np 
import math 
import matplotlib
import pandas as pd
matplotlib.rcParams['pdf.fonttype'] = 42
matplotlib.rcParams['ps.fonttype'] = 42


color_list = ['tab:orange',
            'tab:blue',
            'tab:green',
            'tab:red',
            'tab:purple',
            'tab:brown',
            'tab:pink',
            'tab:gray',
            'tab:olive',
            'tab:cyan']

hatch_list = [
    '', 
    '/', 
    '\\'
    '///', 
    '--', 
    '+', 
    'x'
    '*', 
    'o', 
    'O', 
    '.'
]

line_style_list = [
    '-', 
    '--', 
    '-.', 

]

marker_list = [
    '',
    'o', 
    'v',
    '^', 
    'X', 
    'D'
    's', 
]

template = {
    'fontsize': 18, 
    'linewidth': 6, 
    'scatter_markersize': 400, 
    'line_markersize': 20, 
    'width': 2, 
}

def autolabel_percent(rects, ax, value_list, error_list=None, str_func=None):
    if str_func is None: 
        str_func = lambda x: '%.2f'%(x)

    if error_list is None: 
        error_list = [0 for _ in value_list]

    for idx, rect in enumerate(rects):
        if value_list[idx] is None: continue
        height = rect.get_height()
        ax.annotate(str_func(value_list[idx]),
                    xy=(rect.get_x() + rect.get_width() / 2, height+error_list[idx]),
                    xytext=(0, 3),  # 3 points vertical offset
                    textcoords="offset points",
                    ha='center', va='bottom', fontsize=16, fontweight='bold')


def check_before_run(**kwargs): 
    if   kwargs['full'] + kwargs['half'] + kwargs['forth'] > 1: 
        return False 
    return True 


def apply_grid(ax, **kwargs): 
    ax.grid(linestyle='-', linewidth=1, alpha=0.5, axis='y')
    # if kwargs.get('grid'): 
    #     if not (kwargs.get('ygrid') or kwargs.get('xgrid')): 
    #         ax.grid(linestyle='-.', linewidth=1, alpha=0.5)

    # if kwargs.get('ygrid'): 
    #     ax.grid(linestyle='-.', linewidth=1, alpha=0.5, axis='y')
    # if kwargs.get('xgrid'): 
    #     ax.grid(linestyle='-.', linewidth=1, alpha=0.5, axis='x')


def apply_spine(ax, **kwargs): 
    if kwargs.get('spines'): 
        ax.spines['right'].set_color('none')
        ax.spines['top'].set_color('none')


def apply_font(kwargs): 
    font = {'family' : 'serif',
            'size'   : 18}
    if kwargs.get('font'): 
        font.update(kwargs.get('font'))
    matplotlib.rc('font', **font)


def apply_log(ax, **kwargs): 
    pass 

def init_plot(ncols, **kwargs): 
    # if len(kwargs) > 0: 
    #     assert check_before_run(kwargs)
    
    apply_font(kwargs)
    if isinstance(ncols, tuple): 
        fig, axes = matplotlib.pyplot.subplots(ncols[0], ncols[1])
        fig.set_size_inches(w=ncols[1]* 4*3, h=3*ncols[0])
        
        axes = [axes[i] for i in range(ncols[0] * ncols[1])]
#         axes = [axes[j][i] for i in range(ncols[0]) for j in range(ncols[1])]
        # import pdb; pdb.set_trace() 
    else: 
        fig, axes = matplotlib.pyplot.subplots(1, ncols)
        if ncols == 1: 
            axes = [axes]
        # fig.set_size_inches(w=ncols* 10.8, h=6.3)
        fig.set_size_inches(w=ncols* 6, h=4)

    for ax in axes: 
        apply_grid(ax, **kwargs)
        apply_spine(ax, **kwargs)

    return fig, axes 



root = os.path.dirname(os.path.realpath(__file__))
while not root.endswith('loader-gpt'): 
    root = os.path.dirname(root)


if True: 
    for i in range(5): 
        csv_name = f'data/real-multi-func-gputraces/example/reference_{i}.csv'
        # Read the CSV file
        data = pd.read_csv(csv_name)
        data['submission_time'] = data['submission_time'].apply(lambda x: x // 1)


        # Group the data by submission time and count the number of requests per time unit
        grouped_data = data.groupby('submission_time').size()


        # Plot the bar chart
        # grouped_data.plot(kind='bar', figsize=(8, 6), color='tab:blue')

        y_list = grouped_data.tolist() 
        x_list = [i for i in range(len(y_list))]
        print(i, max(y_list))

csv_name = 'data/real-multi-func-gputraces/example/shai_reference_0.csv'

# Read the CSV file
data = pd.read_csv(csv_name)
data['submission_time'] = data['submission_time'].apply(lambda x: x // 1)


# Group the data by submission time and count the number of requests per time unit
grouped_data = data.groupby('submission_time').size()

num_gpus = data.groupby('submission_time')['num_gpus'].sum()

# Plot the bar chart
# grouped_data.plot(kind='bar', figsize=(8, 6), color='tab:blue')

y_list = grouped_data.tolist() 
y_list = num_gpus.tolist() 
x_list = grouped_data.index.tolist() # [i for i in range(len(y_list))]
print(max(y_list))


template.update(
    {
        "norm": False, 
        "width": 0.3, 
        "autolabel": False, 
        'norm': True,
        'logy': 0,
        'barh': False,
    }
)
new_template =  {
    "norm": True, 
    "width": 0.3, 
    "autolabel": False, 
    'logy': 1,
    'xname': None,
}
template.update(new_template)
fig, axes = init_plot(1, grid=True)

ax = axes[0]
ax.plot(x_list, y_list, color='tab:blue', linewidth=2)

import numpy as np 
print(max(y_list) / np.mean(y_list))
# import pdb; pdb.set_trace() 

fontsize = template['fontsize'] - 8
ax.set_xticks([120 * i for i in range(4)])
ax.set_xticklabels([120 * i for i in range(4)], fontsize=fontsize)
    
# ax.set_yticks([0, 20, 40, 60])
# ax.set_yticklabels([0, 20, 40, 60], fontsize=fontsize)
# ax.set_ylim(0, 60)
ax.set_xlim(0, 360)
# import pdb; pdb.set_trace() 
# Add labels and title


from datetime import datetime, timedelta

print(data['time_zone'].tolist()[0])
given_time = data['time_zone'].tolist()[0]
dt = datetime.strptime(given_time, "%Y-%m-%d %H:%M:%S")
# 取整到小时
hour_dt = dt.replace(minute=0, second=0)

# 生成其他三个时间，每个时间间隔为2小时
time_list = [hour_dt + timedelta(hours=i*2) for i in range(4)]
# import pdb; pdb.set_trace() 
time_str_list = [
    # sub_time.strftime("%Y-%m-%d %H:%M") for sub_time in time_list
    sub_time.strftime("%m-%d %H:%M") for sub_time in time_list
    ]


ax.set_xticks([0, 120, 240, 360])
ax.set_xticklabels(time_str_list, fontsize=template['fontsize']-4, rotation=15)
ax.set_xlabel('Submission Time', fontsize=template['fontsize'])
ax.set_ylabel('Number of Requested GPUs', fontsize=template['fontsize'])
# plt.title('Number of Requested GPUs per Minute')

# Show the plot
plt.savefig(f'{root}/images/client_training/submission_density.jpg', bbox_inches='tight')
print(f'{root}/images/client_training/submission_density.jpg')