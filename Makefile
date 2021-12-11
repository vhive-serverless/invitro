proto:
	protoc \
		--go_out=. \
		--go_opt=paths=source_relative \
		--go-grpc_out=. \
		--go-grpc_opt=paths=source_relative \
		pkg/faas/*.proto 

clean: # make -i clean
	kn service delete --all
	rm -f el
	rm -f *.log
	go mod tidy

build:
	go build cmd/el.go

run:
	go run cmd/el.go