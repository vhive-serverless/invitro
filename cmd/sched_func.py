import os, sys
sys.path.insert(0, './pkg/workload/schedproto')
import time 
from concurrent import futures
import logging
import grpc
import sched_pb2
import sched_pb2_grpc
from dataclasses import dataclass
import logging

# Configure the logging system
logging.basicConfig(format='%(asctime)s - %(levelname)s - %(message)s', 
                    datefmt='%Y/%m/%d %H:%M:%S', level=logging.INFO)


IDLE='idle'
RUNNING='running'
TotalGPU=40
class Empty(object): 
    pass 

@dataclass 
class Job: 
    name: str
    batchsize: int 
    deadline: int 
    iterations: int 
    prevReplica: int 
    

def print_red_text(text):
    RED = "\033[91m"
    RESET = "\033[0m"
    print(RED + text + RESET, flush=True)
          

DEBUG = True 
class Executor(sched_pb2_grpc.Executor):
    def __init__(self, ): 
        super().__init__()
        self.sched_interval = 10 # seconds 
        self.llama_cnt = 0 
        
    def Execute(self, request, context, **kwargs):
        return sched_pb2.SchedReply(replica=1, schedOverhead=1)

    def ExecuteStream(self, request_iterator, context, **kwargs):
        # print("starting running", time.time())
        start = time.time()
        job_infos = list() 
        name_keys = list() 
        remaining_gpus = 0 
        sched_alg = None 
        for request in request_iterator:
            job_infos.append(Job(name=request.invocationName, batchsize=request.batchsize, \
                            deadline=request.deadline, iterations=request.iterations, prevReplica=request.prevReplica))
            name_keys.append(request.invocationName)
            remaining_gpus = request.availableGPU
            sched_alg = request.schedAlg
            
        logging.info(f"sched_alg {sched_alg}, remaining_gpus {remaining_gpus}")
        if sched_alg in ['elastic_flow', 'infless', 'elastic']: 
            num_replicas = {name:0 for name in name_keys}
            desired_replicas = {job.name:job.batchsize // 32 for job in job_infos}
            
            allocation_set = [1, 2] + [i * 4 for i in range(1, TotalGPU//4+1)]
            # scheduler algorithm 
            job_infos = sorted(job_infos, key=lambda job: job.deadline)
            while remaining_gpus > 0 and len(job_infos) > 0: 
                job = job_infos[0]
                allocated_replicas = 0
                for num_replica in allocation_set: 
                    remaining_time = job.batchsize * request.runtimeInMilliSec // 32 * job.iterations // num_replica 
                    deltaCost = 1 * 1e3 if num_replica != job.prevReplica else 0 
                    if remaining_time + deltaCost < job.deadline: 
                        allocated_replicas = num_replica
                        break 
                
                num_replicas[job.name] = allocated_replicas
                
                job_infos.remove(job)
                remaining_gpus -= allocated_replicas
                # if 'gpt2-large' in job.name: 
                logging.info(f'job.name {job.name}, iteration {job.iterations}, job.deadline {job.deadline}, remaining_time {remaining_time}, allocated_replicas {allocated_replicas}')
                # if 'llama' in job.name: 
                #     self.llama_cnt += allocated_replicas
            # print_red_text("llama_cnt replicas {}".format(self.llama_cnt))
            for name in name_keys: 
                if num_replicas[name] == 0 and remaining_gpus > 0: 
                    num_replicas[name] = min(desired_replicas[name], remaining_gpus)
                    remaining_gpus -= num_replicas[name]
                    # if 'llama' in name: 
                    #     self.llama_cnt += num_replicas[name]
            
        ret_replicas = [int(num_replicas[name]) for name in name_keys]
        print(name_keys, flush=True)
        print(ret_replicas, flush=True)
        response = sched_pb2.SchedReply(invocationName=name_keys, replica=ret_replicas, schedOverhead=int(time.time()-start))
        return response 
        # for name in name_keys: 
        #     response = sched_pb2.SchedReply(invocationName=name, replica=num_replicas[name], schedOverhead=int(time.time()-start))
        #     yield response
               
    # async def ExecuteBatch(self, request_iterator, context, **kwargs):
    #     # Collect incoming requests and batch them
    #     requests_batch = []
    #     async for request in request_iterator:
    #         requests_batch.append(request)
        
    #     await asyncio.sleep(2)

    #     # Process the batch of requests and yield individual responses
    #     for batch_request in requests_batch:
    #         total_sum = sum(batch_request.numbers)
    #         yield sched_pb2.SchedReply(action=IDLE, replica=1, schedOverhead=1)
            

def serve():
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=10))
    sched_pb2_grpc.add_ExecutorServicer_to_server(Executor(), server)
    server.add_insecure_port('[::]:50051')
    server.start()
    server.wait_for_termination()

if __name__ == '__main__':
    print('starting sever ...', flush=True)
    logging.basicConfig()
    serve()