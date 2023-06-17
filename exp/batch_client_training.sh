
clean_env() {
    kn service list | awk '{print $1}' | xargs kn service delete 
    sleep 60 
}

for duration in 10 20 40 # 5 # 10 # 20 30 # 20 # 40 60 120 # 40 # 10 20 40 # 60 120 180 #
do
    go run cmd/loader.go --config cmd/config_client_hived.json  --overwrite_duration ${duration}
    clean_env "$@"

    go run cmd/loader.go --config cmd/config_client_single.json  --overwrite_duration ${duration}
    clean_env "$@"

    go run cmd/loader.go --config cmd/config_client_batch.json  --overwrite_duration ${duration}
    clean_env "$@"

    # go run cmd/loader.go --config cmd/config_client_pipeline_batch_priority.json  --overwrite_duration ${duration}
    # clean_env "$@"

    # go run cmd/loader.go --config cmd/config_client_batch_priority.json  --overwrite_duration ${duration}
    # clean_env "$@"

done 