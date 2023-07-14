kn service delete gpttrace
sleep 2 

bash pkg/driver/deploy_gpt_gpu.sh workloads/container/trace_func_gpt_gpu_template.yaml gpttrace \
    2000m 2000m 4000Mi 1 1 1 \"10.0\" \"200.0\" \"concurrency\"  \"100\"
