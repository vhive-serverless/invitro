import grpc
import faas_pb2
import faas_pb2_grpc


# channel = grpc.insecure_channel("localhost:80")
channel = grpc.insecure_channel("gpt.default.10.200.3.4.sslip.io:80")
stub = faas_pb2_grpc.ExecutorStub(channel)


request = faas_pb2.FaasRequest(message='Hello, world!', batchsize=32, runtimeInMilliSec=100, gpuMemoryInMebiBytes=1000, promptTensor=[0 for _ in range(128)])
response = stub.Execute(request)

print(response)
exit(0)

if True: 
    import grpc
    import faas_pb2
    import faas_pb2_grpc
    # exit(0)
    # create a channel to the gRPC server
    channel = grpc.insecure_channel("http://gpt.default.10.200.3.4.sslip.io:80")

    # create a stub for the gRPC service
    stub = faas_pb2_grpc.ExecutorStub(channel)

    # create a request message
    request = faas_pb2.FaasRequest(message='Hello, world!', runtimeInMilliSec=10, memoryInMebiBytes=10)

    # send the request to the gRPC server
    response = stub.Execute(request)

    # print the response message
    print(response)
else:
    import requests

    url = "http://gpt.default.10.200.3.4.sslip.io:80/"

    headers = {
        "Content-Type": "application/json"
    }

    data = {
        "input": "Hello, World!", 
        "runtime":10, 
        "memory":10
    }

    response = requests.post(url, json=data, headers=headers)

    print(response.json())
