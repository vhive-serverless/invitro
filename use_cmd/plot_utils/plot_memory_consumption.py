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
        fig.set_size_inches(w=ncols* 4, h=3)

    for ax in axes: 
        apply_grid(ax, **kwargs)
        apply_spine(ax, **kwargs)

    return fig, axes 



root = os.path.dirname(os.path.realpath(__file__))
while not root.endswith('loader-gpt'): 
    root = os.path.dirname(root)
        
csv_name = 'data/real-multi-func-gputraces/example/reference_0.csv'

# Read the CSV file
data = pd.read_csv(csv_name)
data['submission_time'] = data['submission_time'].apply(lambda x: x // 1)


# Group the data by submission time and count the number of requests per time unit
grouped_data = data.groupby('submission_time').size()


# Plot the bar chart
# grouped_data.plot(kind='bar', figsize=(8, 6), color='tab:blue')

y_list = grouped_data.tolist() 
x_list = [i for i in range(len(y_list))]







inference_info = {
    'Llama-7B': [[1, 2, 4, 8], [12.633418560028076, 12.656738758087158, 12.699684143066406, 12.785574913024902]], 
    'GPT2': [[1, 2, 4, 8], [1.4902386665344238, 1.5089879035949707, 1.543262004852295, 1.6076550483703613] ], 
}

prompt_info = {
    'Llama-7B': [[1, 2, 4, 8], [12.895644664764404, 13.23575210571289, 13.87380838394165, 15.118852615356445]], 
    'GPT2': [[1, 2, 4, 8], [1.6106276512145996, 1.779740333557129, 2.090010643005371, 2.692291736602783]], 
}


for model in ['GPT2', 'Llama-7B']: 
    
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

    bsz_list = inference_info[model][0]
    memory_list = inference_info[model][1]
    ax = axes[0]
    ax.plot(bsz_list, memory_list, color='tab:blue', linewidth=2, label='inference')
    bsz_list = prompt_info[model][0]
    memory_list = prompt_info[model][1]
    ax.plot(bsz_list, memory_list, color='tab:orange', linewidth=2, label='prompt')

    fontsize = template['fontsize'] - 8
    ax.set_xticks(bsz_list)
    ax.set_xticklabels(bsz_list, fontsize=fontsize)
    if 'GPT' in model: 
        ax.set_yticks([1, 1.5, 2, 2.5, 3])
        ax.set_yticklabels([1, 1.5, 2, 2.5, 3], fontsize=fontsize)
    else: 
        ax.set_yticks([12, 13, 14, 15, 16])
        ax.set_yticklabels([12, 13, 14, 15, 16], fontsize=fontsize)
    # 
    # 
    # ax.set_ylim(0, 60)
    # ax.set_xlim(0, 360)
    # import pdb; pdb.set_trace() 
    # Add labels and title
    ax.set_xlabel('Batch Size', fontsize=fontsize)
    ax.set_ylabel('GPU Memory [GB]', fontsize=fontsize)
    ax.legend(fontsize=template['fontsize'] - 4, loc='upper center', ncol=2, bbox_to_anchor=(0.5, 1.2), fancybox=True, shadow=False, edgecolor="white", handlelength=2) 
    # plt.title('Number of Requests per Time Unit')

    # Show the plot
    plt.savefig(f'{root}/images/client_training/{model}.jpg', bbox_inches='tight')
    print(f'{root}/images/client_training/{model}.png')