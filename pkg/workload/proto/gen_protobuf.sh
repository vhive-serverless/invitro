go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.26
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.1
export PATH="$PATH:$(go env GOPATH)/bin"
python3 -m grpc_tools.protoc --proto_path=. --python_out=. --grpc_python_out=. faas.proto
protoc --go_out=. --go-grpc_out=.  faas.proto
mv proto/proto/*.go ./
rm -rf proto/
