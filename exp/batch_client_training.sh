
clean_env() {
    kn service list | awk '{print $1}' | xargs kn service delete 
    sleep 360 
}

for duration in 240 # 20 40 60 80 120 150 # 240 # 80 # 10 20 40 60 80 120 150 # 60 # 20 40 60 # 80 # 120 # 150 180 # 10 20 40 60 80 120 # 120 160 # 80 # 80 #  20 40  # 20 40 # 5 # 10 # 20 30 # 20 # 40 60 120 # 40 # 10 20 40 # 60 120 180 #
do
    # go run cmd/loader.go --config cmd/config_client_hived_elastic.json  --overwrite_duration ${duration} > log/hived_elastic_log_$duration.txt
    # clean_env "$@"

    # go run cmd/loader.go --config cmd/config_client_hived.json  --overwrite_duration ${duration} # > log/hived_log_$duration.txt
    # clean_env "$@"

    # go run cmd/loader.go --config cmd/config_client_single.json  --overwrite_duration ${duration} # > log/single_log_$duration.txt
    # clean_env "$@"

    go run cmd/loader.go --config cmd/config_client_batch.json  --overwrite_duration ${duration} # > log/batch_log_$duration.txt
    clean_env "$@"

    # go run cmd/loader.go --config cmd/config_client_pipeline_batch_priority.json  --overwrite_duration ${duration} > log/single_log_$duration.txt
    # clean_env "$@"

    # go run cmd/loader.go --config cmd/config_client_batch_priority.json  --overwrite_duration ${duration} # > log/single_log_$duration.txt
    # clean_env "$@"

done 