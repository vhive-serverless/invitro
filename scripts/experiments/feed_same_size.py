#!/usr/bin/env python3

import sys
import os
import os.path
from time import sleep

def main(argv):    
    _, duration, cluster, src = argv
    
        
    for i in range(1, 101):
        trace_path = f"data/samples/converge-{src}/{i}/"
        if i > 0:
            os.system('kubectl rollout restart deployment activator -n knative-serving')
            os.system('make -i clean')
            sleep(5)
        try:
            cmd = f"make ARGS='-sample 50 -duration {duration} -cluster {cluster} -server trace -warmup -iatDistribution exponential -tracePath {trace_path}' run 2>&1 | tee converge_{i}.log"
            print(cmd)
            os.system(command=cmd)
        except KeyboardInterrupt:
            print('Experiment interrupted')
            try: sys.exit(0)
            except SystemExit: os._exit(0)  
        
        os.system(command=f"mv data/out data/out-converge_{i}")
        os.system(command=f"mkdir data/out")
        

if __name__ == '__main__':
    main(sys.argv)