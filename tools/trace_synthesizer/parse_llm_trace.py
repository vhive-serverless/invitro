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
        fig.set_size_inches(w=ncols* 10.8, h=6.3)

    for ax in axes: 
        apply_grid(ax, **kwargs)
        apply_spine(ax, **kwargs)

    return fig, axes 



root = os.path.dirname(os.path.realpath(__file__))
while not root.endswith('loader-gpt'): 
    root = os.path.dirname(root)
        
csv_name = 'data/real-multi-func-gputraces/example/llm_it_traces.csv'

# Read the CSV file
df = pd.read_csv(csv_name)

# Convert Unix timestamps to datetime
df['time_submit'] = pd.to_datetime(df['time_submit'], unit='s')
df['time_start'] = pd.to_datetime(df['time_start'], unit='s')
df['time_end'] = pd.to_datetime(df['time_end'], unit='s')

# Filter rows where state is COMPLETED
# import pdb; pdb.set_trace() 
df = df[df['state'] == 'COMPLETED']
df['duration'] = (df['time_end'] - df['time_start']).dt.total_seconds()

df = df[df['duration'] <= 1800]
# df = df[df['duration'] >= 10]
df = df[df['alloc_gpu'] > 0]
df['gpu_time'] = df['alloc_gpu'] * df['duration']
df['new_alloc'] = df.alloc_gpu.apply(lambda x: 1 if x <= 4 else x // 4)


# import pdb; pdb.set_trace() 
# Calculate the timestamp difference in minutes
reference_time = pd.to_datetime('2023-04-18 09:00:00')
# reference_time = pd.to_datetime('2023-04-18 17:00:18')

df['timestamp'] = (df['time_submit'] - reference_time).dt.total_seconds() // 60
if True: 
    df_grouped = df.groupby('timestamp').size().reset_index(name='count')
    x_list = df_grouped['timestamp'].tolist()
    
    y_list = df.groupby('timestamp')['new_alloc'].sum().tolist()
    # import pdb; pdb.set_trace() 
else: 
    df_grouped = df.groupby('timestamp').size().reset_index(name='count')
    x_list = df_grouped['timestamp'].tolist()
    y_list = df_grouped['count'].tolist()


minute_per_hour = 60 
elapsed_minute = minute_per_hour * 6


def extract_trace(df, start_index, end_index):
    # import pdb; pdb.set_trace() 
    extracted_df = df[df['timestamp'] >= start_index]
    extracted_df = extracted_df[extracted_df['timestamp'] <= end_index]
    extracted_df.timestamp = extracted_df.timestamp.apply(lambda index: int(index-start_index))
    extracted_df.duration = extracted_df.duration.apply(lambda duration: int(duration))
    extracted_df = extracted_df[['time_submit', 'timestamp', 'alloc_gpu', 'duration']].copy()
    new_submit_list, new_stamp_list, new_gpu_list, new_duration_list = list(), list(), list(), list() 
    
    submit_list = extracted_df['time_submit'].tolist() 
    stamp_list = [int(x) for x in extracted_df['timestamp'].tolist() ]
    gpu_list = [int(x) for x in extracted_df['alloc_gpu'].tolist() ]
    duration_list = [int(x) for x in extracted_df['duration'].tolist() ]
    for submit, stamp, gpu, duration in zip(submit_list, stamp_list, gpu_list, duration_list): 
        if gpu >= 4: 
            for _ in range(gpu // 4): 
                new_submit_list.append(submit)
                new_stamp_list.append(stamp)
                new_gpu_list.append(4)
                new_duration_list.append(duration)
        else: 
            new_submit_list.append(submit)
            new_stamp_list.append(stamp)
            new_gpu_list.append(gpu)
            new_duration_list.append(duration)
            
    new_df = pd.DataFrame({
        'time_zone': new_submit_list,
        'submission_time': new_stamp_list,
        'num_gpus': new_gpu_list,
        'duration': new_duration_list
    })
    print(sum(gpu_list))
    return new_df

rank = 0 
while rank < 5: 
    start_list, job_cnt_list = list(), list() 
    for start in range(0, len(x_list) - minute_per_hour, 1): 
        number_of_jobs = 0 
        for time_x, count_y in zip(x_list, y_list): 
            if time_x < start: 
                continue 
            if time_x > start + elapsed_minute: 
                break 
            number_of_jobs += count_y 
        start_list.append(start)
        job_cnt_list.append(number_of_jobs)
    
    dense_start = start_list[job_cnt_list.index(max(job_cnt_list))]
    print('max job is {}'.format(max(job_cnt_list)))
    saved_df = extract_trace(df, dense_start, dense_start + elapsed_minute)
    saved_df.to_csv(f'data/real-multi-func-gputraces/example/shai_reference_{rank}.csv')
    print(f'data/real-multi-func-gputraces/example/shai_reference_{rank}.csv')
    for idx, (time_x, count_y) in enumerate(zip(x_list, y_list)): 
        if time_x < dense_start: 
            continue 
        if time_x > dense_start + elapsed_minute: 
            break 
        y_list[idx] = -100
    rank = rank + 1
exit(0)


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

# Show the plot
plt.savefig(f'{root}/images/client_training/submission_density.jpg', bbox_inches='tight')
print(f'{root}/images/client_training/submission_density.jpg')



exit(0)
ref_list = [6, 6, 6, 8, 8, 5, 8, 8] # , 7, 23, 14, 5, 3, 5, 4, 4, 6, 4, 4, 22, 2, 12, 6, 4, 4, 7]
print(y_list)

for idx in range(len(y_list) - len(ref_list)): 
    sub_list = y_list[idx:idx+len(ref_list)]
    found = True 
    for true_y, ref_y in zip(sub_list, ref_list): 
        if true_y != ref_y: 
            found = False 
            break 
    if found: 
        import pdb; pdb.set_trace() 
exit(0)

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

# fontsize = template['fontsize'] - 8
# ax.set_xticks([120 * i for i in range(4)])
# ax.set_xticklabels([120 * i for i in range(4)], fontsize=fontsize)
    
# ax.set_yticks([0, 20, 40, 60])
# ax.set_yticklabels([0, 20, 40, 60], fontsize=fontsize)
# ax.set_ylim(0, 60)
# ax.set_xlim(0, 360)
# import pdb; pdb.set_trace() 
# Add labels and title
# ax.set_xlabel('Submission Time (min)', fontsize=template['fontsize'])
# ax.set_ylabel('Number of Requests', fontsize=template['fontsize'])
# plt.title('Number of Requests per Time Unit')

# Show the plot
plt.savefig(f'{root}/images/client_training/submission_density.jpg', bbox_inches='tight')
print(f'{root}/images/client_training/submission_density.jpg')