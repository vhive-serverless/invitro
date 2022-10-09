.PHONY : proto clean build run trace-firecracker trace-container wimpy

proto:
	protoc \
		--go_out=. \
		--go_opt=paths=source_relative \
		--go-grpc_out=. \
		--go-grpc_opt=paths=source_relative \
		server/faas.proto 
	/usr/bin/python3 -m grpc_tools.protoc -I=. \
		--python_out=. \
		--grpc_python_out=. \
		server/faas.proto

# make -i clean
clean: 
# 	kubectl rollout restart deployment activator -n knative-serving
	kubectl rollout restart statefulset prometheus-prometheus-kube-prometheus-prometheus -n monitoring
	kn service delete --all
	kubectl delete --all all -n default --grace-period=0 

# 	Deployments should be deleted first!
# 	kubectl delete --all deployments,pods,podautoscalers -n default
# 	kubectl delete --all deployments -n default
# 	kubectl delete --all pods -n default
# 	kubectl delete --all podautoscalers -n default

	bash scripts/warmup/reset_kn_global.sh
	rm -f load
# 	rm -f *.log
	go mod tidy

logs:
	mkdir logs
	mv *.log *.flag logs

rm-results:
	rm *.log *.flag data/out/*

build:
	go build cmd/load.go

# make ARGS='--rps X --duration X' run 2>&1 | tee loader.log
run:
	go run cmd/load.go $(ARGS)

test:
	go test -v -cover -race ./pkg/generate/
	go test -v -cover -race ./pkg/test/

# Used for replying the trace
trace-firecracker:
	docker build --build-arg FUNC_TYPE=TRACE \
		--build-arg FUNC_PORT=50051 \
		-f Dockerfile.trace \
		-t cvetkovic/trace_function_firecracker .
	docker push cvetkovic/trace_function_firecracker:latest

# Used for replying the trace
trace-container:
	docker build --build-arg FUNC_TYPE=TRACE \
		--build-arg FUNC_PORT=80 \
		-f Dockerfile.trace \
		-t cvetkovic/trace_function .
	docker push cvetkovic/trace_function:latest

# Used for measuring cold start latency
empty-firecracker:
	docker build --build-arg FUNC_TYPE=EMPTY \
		--build-arg FUNC_PORT=50051 \
		-f Dockerfile.trace \
		-t cvetkovic/empty_function_firecracker .
	docker push cvetkovic/empty_function_firecracker:latest

# Used for measuring cold start latency
empty-container:
	docker build --build-arg FUNC_TYPE=EMPTY \
		--build-arg FUNC_PORT=80 \
		-f Dockerfile.trace \
		-t cvetkovic/empty_function .
	docker push cvetkovic/empty_function:latest

wimpy:
	docker build -f Dockerfile.wimpy -t hyhe/wimpy .
	docker push hyhe/wimpy:latest

