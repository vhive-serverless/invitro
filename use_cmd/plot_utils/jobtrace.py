
import os, sys 

class JobTrace(object): 
    def __init__(self, filename): 
        with open(filename, 'r') as f: 
            lines = f.readlines()
        self.job_dict = dict() 
        for line in lines:
            if len(line) > 2: 
                job_info  = eval(line) 
                self.job_dict[job_info['invocationID']] = job_info 
    
    def get_replica(self, invocation_id):
        if invocation_id in self.job_dict:
            return self.job_dict[invocation_id]['replica']
        else:
            return None

    def get_execution_time(self, invocation_id):
        if invocation_id in self.job_dict:
            return self.job_dict[invocation_id]['executionTime']
        else:
            return None

    def get_compute_time(self, invocation_id):
        if invocation_id in self.job_dict:
            return self.job_dict[invocation_id]['computeTime']
        else:
            return None

    def get_gpu_count(self, invocation_id):
        if invocation_id in self.job_dict:
            return self.job_dict[invocation_id]['gpuCount']
        else:
            return None
        
    def get_start_teration(self, invocation_id):
        if invocation_id in self.job_dict:
            return self.job_dict[invocation_id]['startIteration']
        else:
            return None

    def get_end_teration(self, invocation_id):
        if invocation_id in self.job_dict:
            return self.job_dict[invocation_id]['endIteration']
        else:
            return None