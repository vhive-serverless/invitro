.PHONY : proto clean build run trace-firecracker trace-container wimpy

proto:
	protoc \
		--go_out=. \
		--go_opt=paths=source_relative \
		--go-grpc_out=. \
		--go-grpc_opt=paths=source_relative \
		pkg/workload/proto/faas.proto pkg/workload/proto/nexus_rpc.proto \
		pkg/workload/proto/knative_integration.proto
	/usr/bin/python3 -m grpc_tools.protoc -I=. \
		--python_out=. \
		--grpc_python_out=. \
		pkg/workload/proto/faas.proto

# make -i clean
clean: 
# 	kubectl rollout restart deployment activator -n knative-serving

	scripts/util/clean_prometheus.sh

	kn service delete --all
	kubectl delete --all all -n default --grace-period=0 

# 	Deployments should be deleted first!
# 	kubectl delete --all deployments,pods,podautoscalers -n default
# 	kubectl delete --all deployments -n default
# 	kubectl delete --all pods -n default
# 	kubectl delete --all podautoscalers -n default

	bash scripts/warmup/reset_kn_global.sh
	rm -f loader
# 	rm -f *.log
	go mod tidy

rm-results:
	rm data/out/*.csv

build:
	go build cmd/loader.go

run:
	go run cmd/loader.go --config cmd/config_knative_trace.json

test:
	go test -v -cover -race \
		./pkg/config/ \
		./pkg/driver/ \
		./pkg/generator/ \
		./pkg/trace/

# Used for replying the trace
trace-firecracker:
	docker build --build-arg FUNC_TYPE=TRACE \
		--build-arg FUNC_PORT=50051 \
		-f Dockerfile.trace \
		-t ghcr.io/vhive-serverless/invitro_trace_function_firecracker .
	docker push ghcr.io/vhive-serverless/invitro_trace_function_firecracker:latest

# Used for replying the trace
trace-container:
	docker build --build-arg FUNC_TYPE=TRACE \
		--build-arg FUNC_PORT=80 \
		-f Dockerfile.trace \
		-t ghcr.io/vhive-serverless/invitro_trace_function .
	docker push ghcr.io/vhive-serverless/invitro_trace_function:latest

# Used for measuring cold start latency
empty-firecracker:
	docker build --build-arg FUNC_TYPE=EMPTY \
		--build-arg FUNC_PORT=50051 \
		-f Dockerfile.trace \
		-t ghcr.io/vhive-serverless/invitro_empty_function_firecracker:latest .
	docker push ghcr.io/vhive-serverless/invitro_empty_function_firecracker:latest

# Used for measuring cold start latency
empty-container:
	docker build --build-arg FUNC_TYPE=EMPTY \
		--build-arg FUNC_PORT=80 \
		-f Dockerfile.trace \
		-t ghcr.io/vhive-serverless/invitro_empty_function:latest .
	docker push ghcr.io/vhive-serverless/invitro_empty_function:latest

wimpy:
	docker build -f Dockerfile.wimpy -t hyhe/wimpy .
	docker push hyhe/wimpy:latest
