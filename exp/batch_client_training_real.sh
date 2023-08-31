
clean_env() {
    kn service list | awk '{print $1}' | xargs kn service delete 
    sleep 120 
}

for duration in 5 10 # 20 # 20 30 # 10 # 5 10 # 20 # 30 40 60 80 120 150 240 # 10 20 30 40 60 # 80 120 150 240
do
    for load in 0.3 0.5 0.7 0.9 #  0.3 0.4 0.5 0.6 0.7 0.8 
    do 
        # rm log/hived_elastic_log_$duration.txt
        go run cmd/loader.go --config cmd/real_configs/config_client_hived_elastic_real-${load}.json  \
                            --overwrite_duration ${duration} 2>&1 | tee -a log/hived_elastic_log_${duration}_${load}.txt
        clean_env "$@"
        
        go run cmd/loader.go --config cmd/real_configs/config_client_batch_real-${load}.json  \
                            --overwrite_duration ${duration} # > log/batch_log_$duration.txt
        clean_env "$@"
    done 
done 
