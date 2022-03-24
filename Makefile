.PHONY : proto clean build run coldstart trace-func busy-wait sleep

trace-func:
	docker build -f Dockerfile.trace -t hyhe/trace-func-go .
	docker push hyhe/trace-func-go:latest

busy-wait:
	docker build -f Dockerfile.busy -t hyhe/busy-wait .
	docker push hyhe/busy-wait:latest

sleep:
	docker build -f Dockerfile.sleep -t hyhe/sleep .
	docker push hyhe/sleep:latest

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
	kn service delete --all
	kubectl delete --all pods --namespace=default
	bash scripts/warmup/reset_kn_global.sh
	kubectl rollout restart deployment activator -n knative-serving
	rm -f load
# 	rm -f *.log
	go mod tidy

rm-data:
	rm *.log *.flag data/out/*

build:
	go build cmd/load.go

# make ARGS='--rps X --duration X' run 2>&1 | tee loader.log
run:
	go run cmd/load.go $(ARGS)

test:
	go test ./pkg/test/ -v 

# coldstart: clean run