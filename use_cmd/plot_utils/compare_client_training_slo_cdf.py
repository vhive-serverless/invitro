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
    'fontsize': 18 + 4+4, 
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
    if kwargs.get('grid'): 
        if not (kwargs.get('ygrid') or kwargs.get('xgrid')): 
            ax.grid(linestyle='-.', linewidth=1, alpha=0.5)

    if kwargs.get('ygrid'): 
        ax.grid(linestyle='-.', linewidth=1, alpha=0.5, axis='y')
    if kwargs.get('xgrid'): 
        ax.grid(linestyle='-.', linewidth=1, alpha=0.5, axis='x')


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
        fig.set_size_inches(w=ncols[1]* 5, h=3*ncols[0])
        
        axes = [axes[i] for i in range(ncols[0] * ncols[1])]
        # axes = [axes[j][i] for i in range(ncols[0]) for j in range(ncols[1])]
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


def cal_jct(df): 
    num_job = 1.0 * len(df)
    jct = 0
    min_time = sys.maxsize
    max_time = 0
    jct_list = list() 
    # print('num of jobs == {}'.format(num_job))
    for idx, job in df.iterrows(): 
        jct += job.responseTime / num_job
        # jct += job.actualDuration / num_job 
        min_time = min(job.startTime, min_time)
        max_time = max(job.startTime+job.responseTime, max_time)
        jct_list.append(job.responseTime)
    return jct, max(jct_list) # max_time - min_time



def cal_dmr(df): 
    num_job = 1.0 * len(df)
    miss_jobs = 0 
    print('responseTime', 'iteration', 'deadline', 'replicas')
    for idx, job in df.iterrows(): 
        if job.responseTime > job.deadline: 
            miss_jobs += 1
            # import pdb; pdb.set_trace()
            # print('--')
        # print(job.responseTime/1e3, job.iteration/10, job.deadline/1e3, job.batchsize / 32)
    return miss_jobs / num_job 

def cal_fft(df): 
    unfairs = 0 
    for idx, job in df.iterrows(): 
        if job.finish_time_fairness > 1: 
            unfairs += 1
    num_job = len(df)
    return unfairs / num_job

def plot_bar_by_method(ax, info_by_method, **kwargs): 
    apply_log(ax, **kwargs)
    width = kwargs.get('width', 0.3)
    interval = int(math.ceil(width * len(info_by_method)) * 1.4)
    if kwargs.get('norm'): 
        norm_list = list() 
    
    for idx, (ident, y_list, error_list) in enumerate(info_by_method): 
        x_list = list() 
        base = width * ( (len(info_by_method) - 1) // 2 + 0.5 * (len(info_by_method) - 1) % 2 ) + idx * width
        value_list = list() 
        for idy, y in enumerate(y_list): 
            x_list.append(base + idy * interval)
            value_list.append(y)


        if kwargs.get('norm'): 
            if len(norm_list) > 0: 
                pass
                # print(ident)
                # print(norm_list, len(norm_list))
                # print(value_list, len(value_list))
            if len(norm_list) == 0: 
                norm_list = [val for val in value_list]
                value_list = [1. for _ in value_list]
            else: 
                value_list = [round(val / norm, 2) for val, norm in zip(value_list, norm_list)]
                print('ident {}, value_list {}'.format(ident, value_list))
                # import pdb; pdb.set_trace() 
        # import pdb; pdb.set_trace() 
        if error_list is None: 
            error_list = [0 for _ in y_list]
        
        def cap(value): 
            return value 
        
        value_list = [cap(value) for value in  value_list]
        for idy, (x_item, value_item) in enumerate(zip(x_list, value_list)): 
            if kwargs.get('disable_legend') or idy > 0: 
                rect = ax.bar([x_item], [value_item], width=width, color=color_list[idy], hatch=hatch_list[idx], alpha=0.75, edgecolor='black', capsize=0)
            else: 
                # if ident == 'single': 
                #     ident = 'w/o comm'
                # if ident == 'batch': 
                #     ident = 'with comm'
                rect = ax.bar([x_item], [value_item], width=width, color='w', hatch=hatch_list[idx], alpha=0.75, edgecolor='black', capsize=0, label=ident)
                rect = ax.bar([x_item], [value_item], width=width, color=color_list[idy], hatch=hatch_list[idx], alpha=0.75, edgecolor='black', capsize=0)
                
        # print('x_list', x_list)
        # print('y_list', y_list)
        if kwargs.get('autolabel'): 
            #autolabel_percent(rects, ax, value_list, error_list=None, str_func=None):
            str_func = None 
            if kwargs.get('norm'): 
                str_func = lambda x: '%.2f'%(x)
            elif 'int' in str(type(y_list[0])):
                str_func = lambda x: '%d'%(x)
            # print('ident {}, value_list {}'.format(ident, value_list))
            # autolabel_percent(rect, ax, value_list, error_list=error_list, str_func=str_func)
        ax.set_xticks([])
        ax.set_xticks([], minor=True)
        
if True: 
    root = os.path.dirname(os.path.realpath(__file__))
    while not root.endswith('loader-gpt') and not root.endswith('loader'): 
        # print(root)
        root = os.path.dirname(root)
    
    
    duration_list = [5, 10] # , 20] # , 10, 20] # , 30, 40]
    # duration_list = [5]
    
    print(duration_list) # 'perfect', 
    
    
    for duration in duration_list:
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
            "norm": False, 
            "width": 0.3, 
            "autolabel": False, 
            'logy': 1,
            'xname': None,
        }
        template.update(new_template)
        # load_list = [0.5, 0.6, 0.7]
        load_list = [0.3, 0.5, 0.7]
        fig, axes = init_plot((1, len(load_list)), grid=True)
        
        method_list = ['elastic', 'batch']
        # method_list = ['elastic']
        for load_idx, jobload in enumerate(load_list): 
            name_list = None 
            ax = axes[load_idx]
            for method_idx, method in enumerate(method_list): 
                
                method_ident = method if method != 'perfect' else method_list[-1] # 'batch'
                csv_name = os.path.join(root, 'data', 'out', f'real-multi-experiment-jobload-{jobload}_duration_{duration}_ClientTraining_{method_ident}.csv')
                df = pd.read_csv(csv_name)
                print(csv_name)
                if method != 'perfect': 
                    # print(f'processing {method}, job load {jobload}, duration {duration}')
                    df = df[df.requestedDuration > 0]
                    # df = df[df.actualDuration > 0]
                    print(f'processing {method}, job load {jobload}, duration {duration}, job length {len(df)}')
                    job_name_list = df.invocationID.tolist() 
                    job_jct_list = df.responseTime.tolist() 
                    job_duration_list = df.actualDuration.tolist() 
                    job_ddl_list = df.deadline.tolist() 
                    if name_list is None: 
                        all_job_info = [(responseTime, deadline, name) for responseTime, deadline, name in zip(job_jct_list, job_ddl_list, job_name_list)]
                        new_all_job_info = sorted(all_job_info)
                        name_list = [job_info[-1] for job_info in new_all_job_info]
                    # import pdb; pdb.set_trace() 
                    jct_y_list = list() 
                    compute_y_list = list() 
                    ddl_y_list = list() 
                    x_list = list() 
                    for idx, job_name in enumerate(name_list): 
                        index = job_name_list.index(job_name)
                        jct_y_list.append(job_jct_list[index])
                        compute_y_list.append(job_duration_list[index])
                        ddl_y_list.append(job_ddl_list[index])
                        x_list.append(idx)
                    if load_idx == 0: 
                        ax.plot(x_list, jct_y_list, color=color_list[method_idx*3], label=f'{method}-jct')
                        
                        # ax.plot(x_list, ddl_y_list, color=color_list[-1], label=f'{method}-ddl')
                    else: 
                        ax.plot(x_list, jct_y_list, color=color_list[method_idx*3])
                        
                        # ax.plot(x_list, ddl_y_list, color=color_list[-1])
                    if 'elastic' in method: 
                        if load_idx == 0:
                            ax.plot(x_list, compute_y_list, color=color_list[method_idx*3+1], label=f'{method}-comp')
                            ax.plot(x_list, ddl_y_list, color=color_list[-1], label=f'{method}-ddl')
                        else: 
                            ax.plot(x_list, compute_y_list, color=color_list[method_idx*3+1])
                            ax.plot(x_list, ddl_y_list, color=color_list[-1])
                        
        
            if load_idx > 0: 
                template['disable_legend'] = True 
                
            ax.set_xticks([(i + 0.5) * len(method_list) * 0.5  for i in range(len(duration_list))])
            ax.set_xticklabels(duration_list)
            if load_idx == 0: 
                ax.set_ylabel('Runtime')
            ax.set_xlabel(f'load = {jobload}')
        
        fig.legend(fontsize=template['fontsize'] - 8, loc='upper center', ncol=3, bbox_to_anchor=(0.5, 1.2), fancybox=True, shadow=False, edgecolor="white", handlelength=2) 
        plt.savefig(f'{root}/images/client_training/cdf_ddl_{duration}.jpg', bbox_inches='tight')
        print(f'{root}/images/client_training/cdf_ddl_{duration}.jpg')
    
