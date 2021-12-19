.PHONY : proto clean build run coldstart image

image:
	docker build -t hyhe/trace-func-go .
	docker push hyhe/trace-func-go:latest

proto:
	protoc \
		--go_out=. \
		--go_opt=paths=source_relative \
		--go-grpc_out=. \
		--go-grpc_opt=paths=source_relative \
		pkg/faas/*.proto 

# make -i clean
clean: 
	kn service delete --all
	rm -f el
	rm -f *.log
	go mod tidy

build:
	go build cmd/el.go

# make ARGS="--rps X --duration X" run
run:
	go run cmd/el.go $(ARGS)

coldstart: clean run