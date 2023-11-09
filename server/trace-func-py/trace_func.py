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
import grpc
import argparse
import socket
from os import getenv, urandom
from time import process_time, perf_counter
from math import sqrt
import random

import mocks3

import faas_pb2
import faas_pb2_grpc

class Executor(faas_pb2_grpc.Executor):
    def __init__(self, config) -> None:
        super().__init__()
        self.functype = config["functype"]
        self.iterations_multiplier = config["iterations_multiplier"]
        self.hostname = config["hostname"]

    def Execute(self, request, context, **kwargs):
        start_time = perf_counter()
        
        s3_server_list = ["10.0.1.4", "10.0.1.5", "10.0.1.6"]
        s3_server_address = random.choice(s3_server_list)
        s3_port = 60001
        self.bucket_name = "bucket_random"
        self.s3 = mocks3.client(s3_server_address, s3_port)
        
        if self.functype == "EMPTY":
            response = f"OK - EMPTY - {self.hostname}"
        else:
            
            recv = self.s3.download_random(int(request.memoryInMebiBytes)) # Metrics are sent as bytes
            start_time_process = process_time()
            response = self.execute_function(self.hostname, start_time_process, request.runtimeInMilliSec)
            rtt = self.s3.upload_random(int(request.memoryInMebiBytes))  # Download 1MB
        elapsed_second = perf_counter() - start_time
        elapsed_us = int(round(elapsed_second * 1000000))
        return faas_pb2.FaasReply(message=str(response), durationInMicroSec=elapsed_us, memoryUsageInKb=request.memoryInMebiBytes*1024)

    def execute_function(self, hostname, start_time_process, time_to_run_milisecond):
        time_consumed_seconds = process_time() - start_time_process
        if time_consumed_seconds < (time_to_run_milisecond/1000):
            busy_spin_until = start_time_process + time_to_run_milisecond/1000
            tmp = self.busy_spin(busy_spin_until)
        return f"OK - {self.hostname}"

    def busy_spin(self, busy_spin_until: float):
        tmp = 0.0
        while process_time() < busy_spin_until:
            for _ in range(self.iterations_multiplier):
                tmp = self.take_sqrts()
        return tmp

    def take_sqrts(self):
        tmp = random.random()+1
        tmp += sqrt(tmp)
        return tmp

def serve(port: int, functype: str):
    iterations_multiplier = int(getenv("ITERATIONS_MULTIPLIER", "102"))
    hostname = socket.gethostname()
    config = {  "functype": functype, 
                "iterations_multiplier": iterations_multiplier, 
                "hostname": hostname}
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=1))
    faas_pb2_grpc.add_ExecutorServicer_to_server(Executor(config), server)
    server.add_insecure_port(f'[::]:{port}')
    server.start()
    server.wait_for_termination()

if __name__ == '__main__':
    logging.basicConfig(level=logging.INFO)
    parser = argparse.ArgumentParser()
    parser.add_argument('-p', '--port', type=int, default=8081)
    parser.add_argument('-t', '--type', type=str, default="TRACE")
    args = parser.parse_args()
    serve(args.port, args.type)
