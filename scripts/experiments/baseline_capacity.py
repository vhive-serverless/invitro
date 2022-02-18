from glob import glob 
import ntpath
import sys
import os
import os.path

if __name__ == "__main__":
    _, duration, cluster = sys.argv

    tracef = list(map(ntpath.basename, sorted(glob('data/traces/*.csv'))))
    sizes = []
    for f in tracef[::3]:
        sizes.append(int(f.split('_')[0]))
    sizes.sort()

    flagf = 'overload.flag'
    if glob(flagf): os.system(f"rm {flagf}")

    for size in sizes:
        command = f"make ARGS='--sample {size} --duration {duration} --cluster {cluster} --warmup' run 2>&1 | tee cap_{size}.log"
        print(command)
        os.system(command=command)
        if glob(flagf):
            sys.exit()