
clean_env() {
    kn service list | awk '{print $1}' | xargs kn service delete 
    sleep 60 
}

for duration in 5 10 20 40 60 120
do
    for load in 0.3 0.5 0.7 
    do 
        # TODO: (1) elastic -> our proposed elastic and preemptive scheduler 
        # TODO: (2) infless -> infless with slo support 
        # TODO: (3) caerus  -> caerus aims for JCT and cost optimal 
        # TODO: (4) knative -> for single GPU execution with long duration 
        # TODO: (5) optimus -> serverful elastic schedulers 

        for method in elastic # elastic_flow infless # elastic_flow
        do 
            rm log/${method}_log_${duration}_${load}.txt
            go run cmd/loader.go --config cmd/real_configs/config_client_${method}_real-${load}.json  \
                                --overwrite_duration ${duration} # 2>&1 | tee -a log/${method}_log_${duration}_${load}.txt
            
	        clean_env "$@"
            result=$(kubectl get pods)
            while [[ $result == *"-gpu-"* ]] ; do 
                clean_env "$@" 
                result=$(kubectl get pods)
            done 
            
            # exit 
        done 
        
    done 
done 
