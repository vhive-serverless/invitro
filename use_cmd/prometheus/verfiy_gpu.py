import os, sys 
import subprocess 
tot_gpu = 0 
for key, gpu in zip(['gpu-1-', 'gpu-2-', '-gpu-4-'], [1, 2, 4]): 
    command = f'kubectl get pods | grep \"{key}\" | grep -E \"Running|ContainersReady\" | wc -l'
    # print(command)
    result = subprocess.run(command, shell=True, capture_output=True, text=True).stdout
    if len(result) > 0: 
        tot_gpu += gpu * int(result)
    

with open('append.txt', 'a') as f: 
    f.write(f'total used gpu is {tot_gpu}\n')
print(f'total used gpu is {tot_gpu}')
    