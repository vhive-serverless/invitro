#!/usr/bin/env python3

from glob import glob 
import ntpath
import sys
import os
import os.path
from time import sleep

def main(argv):
    _, duration, cluster, server = argv
    if server not in ['trace', 'wimpy']:
        print(f"Function server '{server}' not found")
        sys.exit(1)
    
    repeat = 1 # Repeat the experiements

    tracef = list(map(ntpath.basename, sorted(glob('data/traces/10-1k/*.csv'))))
    sizes = []
    for f in tracef[::3]: #* Duplicated sizes (3 times).
        sizes.append(int(f.split('_')[0]))
    sizes.sort()

    flagf = 'overload.flag'
    if glob(flagf): os.system(f"rm {flagf}")

    for i, size in enumerate(sizes):
        if i > 0:
            os.system('kubectl rollout restart deployment activator -n knative-serving')
            os.system('make -i clean')
            sleep(5)
        try:
            cmd = f"make ARGS='-sample {size} -duration {duration} -cluster {cluster} -server {server} -warmup -iatDistribution exponential' run 2>&1 | tee cap_{size}.log"
            print(cmd)
            os.system(command=cmd)
            if glob(flagf): break
        except KeyboardInterrupt:
            print('Experiment interrupted')
            try: sys.exit(0)
            except SystemExit: os._exit(0)
        
        

if __name__ == '__main__':
    main(sys.argv)