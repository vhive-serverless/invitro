kn service delete gpttrace
sleep 2 

# bash pkg/driver/deploy_gpt.sh workloads/container/trace_func_gpt_gpu_template.yaml gpttrace \
#     4000m 4000m 8000Mi 1 1 1 \"10.0\" \"200.0\" \"concurrency\"  \"100\"


bash pkg/driver/deploy_gpt.sh workloads/container/trace_func_gpt.yaml gpttrace \
    4000m 4000m 8000Mi 1 1 1 \"10.0\" \"200.0\" \"concurrency\"  \"100\"

# kubectl get pods -n kube-system | grep nvidia  | awk '{print $1}' | head -n 1 | xargs -I {} kubectl logs -f {} -n kube-system 
