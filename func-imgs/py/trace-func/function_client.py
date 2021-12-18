from __future__ import print_function
import logging

import grpc

import faas_pb2
import faas_pb2_grpc


def run():
    # NOTE(gRPC Python Team): .close() is possible on a channel and should be
    # used in circumstances in which the with statement does not fit the needs
    # of the code.
    with grpc.insecure_channel('localhost:80') as channel:
        stub = faas_pb2_grpc.ExecutorStub(channel)
        response = stub.Execute(faas_pb2.FaasRequest(input='something'))
    print("Received latency: " + str(response.latency))
    print("Received response: " + str(response.response))


if __name__ == '__main__':
    logging.basicConfig()
    run()
