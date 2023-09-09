
clean_env() {
    kn service list | awk '{print $1}' | xargs kn service delete 
    sleep 120 
}

mkdir -p log 
for duration in 40 60 80 # 5 10 20 # 5 10 20 # 5 # 10 20 # 20 # 20 30 # 10 # 5 10 # 20 # 30 40 60 80 120 150 240 # 10 20 30 40 60 # 80 120 150 240
do
    for load in 0.7 # 0.3 0.5 0.7 # 0.3 0.5 0.7 0.9 #  0.3 0.4 0.5 0.6 0.7 0.8 
    do 
        # TODO: (1) elastic -> our proposed elastic and preemptive scheduler 
        # TODO: (2) infless -> infless with slo support 
        # TODO: (3) caerus  -> caerus aims for JCT and cost optimal 
        # TODO: (4) knative -> for single GPU execution with long duration 
        # TODO: (5) optimus -> serverful elastic schedulers 

        for method in optimus # elastic # knative # optimus # elastic infless caerus knative optimus 
        do 
            rm log/${method}_log_${duration}_${load}.txt
            # if [[ $method == "optimus" ]]; then 
            #     python cmd/sched_func.py &
            #     PID=$!
            # fi 
            
            go run cmd/loader.go --config cmd/real_configs/config_client_${method}_real-${load}.json  \
                                --overwrite_duration ${duration} # 2>&1 | tee -a log/${method}_log_${duration}_${load}.txt
            
            # if [[ $method == "optimus" ]]; then 
            #     kill $PID
            # fi 

            clean_env "$@"
        done 

        
    done 
done 
