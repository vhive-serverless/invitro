#  MIT License
#
#  Copyright (c) 2023 EASL and the vHive community
#
#  Permission is hereby granted, free of charge, to any person obtaining a copy
#  of this software and associated documentation files (the "Software"), to deal
#  in the Software without restriction, including without limitation the rights
#  to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
#  copies of the Software, and to permit persons to whom the Software is
#  furnished to do so, subject to the following conditions:
#
#  The above copyright notice and this permission notice shall be included in all
#  copies or substantial portions of the Software.
#
#  THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
#  IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
#  FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
#  AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
#  LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
#  OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
#  SOFTWARE.

from concurrent import futures
import logging
import datetime
import grpc
import math
from os import getenv
from time import process_time_ns
from random import seed, randrange
from psutil import virtual_memory
from numpy import empty, float32

import faas_pb2
import faas_pb2_grpc

class Executor(faas_pb2_grpc.Executor):

    def Execute(self, request, context, **kwargs):
        start_time = datetime.datetime.now()
        response = execute_function(request.input, request.runtime, request.memory)
        elapsed = datetime.datetime.now() - start_time
        elapsed_us = int(1000000 * elapsed.total_seconds())
        return faas_pb2.FaasReply(latency=elapsed_us, response=str(response))


def execute_function(input, runTime, totalMem):
    startTime = process_time_ns()

    chunkSize = 2**10 # size of a kb or 1024
    totalMem = totalMem*(2**10) # convert Mb to kb
    memory = virtual_memory()
    used = (memory.total - memory.available) // chunkSize # convert to kb
    additional = max(1, (totalMem - used))
    array = empty(additional*chunkSize, dtype=float32) # make an uninitialized array of that size, uninitialized to keep it fast
    # convert to ns
    runTime = (runTime - 1)*(10**6) # -1 because it should be slighly bellow that runtime
    memoryIndex = 0
    while process_time_ns() - startTime < runTime:
        for i in range(0, chunkSize):
            sin_i = math.sin(i)
            cos_i = math.cos(i)
            sqrt_i = math.sqrt(i)
            array[memoryIndex + i] = sin_i
        memoryIndex = (memoryIndex + chunkSize) % additional*chunkSize
    return (process_time_ns() - startTime) // 1000

def serve():
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=1))
    faas_pb2_grpc.add_ExecutorServicer_to_server(Executor(), server)
    server.add_insecure_port('[::]:80')
    server.start()
    server.wait_for_termination()


if __name__ == '__main__':
    logging.basicConfig()
    serve()
