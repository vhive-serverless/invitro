#!/usr/bin/env python3

from glob import glob 
import ntpath
import sys
import os
import os.path

def main(argv):
    _, duration, cluster, server = argv
    if server not in ['trace', 'wimpy']:
        print(f"Function server '{server}' not found")
        sys.exit(1)
    
    repeat = 1 # Repeat the experiements

    tracef = list(map(ntpath.basename, sorted(glob('data/traces/*.csv'))))
    sizes = []
    for f in tracef[::3]:
        sizes.append(int(f.split('_')[0]))
    sizes.sort()

    flagf = 'overload.flag'
    if glob(flagf): os.system(f"rm {flagf}")

    for size in sizes:
        # for _ in range(repeat):
        os.system('make -i clean')
        
        command = f"make ARGS='-sample {size} -duration {duration} -cluster {cluster} -server {server} -warmup' run 2>&1 | tee cap_{size}.log"
        print(command)
        os.system(command=command)
        # if glob(flagf):
        #     break

if __name__ == '__main__':
    try:
        main(sys.argv)
    except KeyboardInterrupt:
        print('Experiment interrupted')
        try:
            sys.exit(0)
        except SystemExit:
            os._exit(0)