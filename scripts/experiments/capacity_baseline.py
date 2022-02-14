from glob import glob 
import ntpath
import sys
import os
import os.path

if __name__ == "__main__":
    duration = sys.argv[1]

    tracef = list(map(ntpath.basename, sorted(glob('data/traces/*.csv'))))
    sizes = []
    for f in tracef[::3]:
        sizes.append(int(f.split('_')[0]))
    sizes.sort()

    flagf = 'overload.flag'
    for size in sizes:
        # print(f"make ARGS='--sample {size} --duration {duration} --withWarmup 2>&1 | tee cap_{size}.log")
        os.system(f"make ARGS='--sample {size} --duration {duration} --warmup' run 2>&1 | tee cap_{size}.log")
        if not glob(flagf):
            continue
        else:
            os.system(f"rm {flagf}")
            break