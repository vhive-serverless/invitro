.PHONY : proto clean build run trace-firecracker trace-container idle

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
	kubectl rollout restart deployment activator -n knative-serving
	kn service delete --all
	kubectl delete --all all -n default --grace-period=0 

	# Deployments should be deleted first!
	# kubectl delete --all deployments,pods,podautoscalers -n default
	# kubectl delete --all deployments -n default
	# kubectl delete --all pods -n default
	# kubectl delete --all podautoscalers -n default

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
	go test ./pkg/test/ -v 

trace-firecracker:
	docker build -f Dockerfile.trace.firecracker -t hyhe/trace-func-firecracker .
	docker push hyhe/trace-func-firecracker:latest

trace-container:
	docker build -f Dockerfile.trace.container -t hyhe/trace-func-container .
	docker push hyhe/trace-func-container:latest

idle:
	docker build -f Dockerfile.idle -t hyhe/idle .
	docker push hyhe/idle:latest
