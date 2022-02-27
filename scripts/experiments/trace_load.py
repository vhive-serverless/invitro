from glob import glob 
import ntpath
import sys
import os
import os.path

def main(argv):
    _, duration, cluster = argv

    sizes = [30, 170, 240, 310, 340, 300, 380, 420, 230, 390, 510]

    flagf = 'overload.flag'
    if glob(flagf): os.system(f"rm {flagf}")

    for size in sizes:
        os.system('make -i clean')
        
        command = f"make ARGS='--sample {size} --duration {duration} --cluster {cluster} --warmup' run 2>&1 | tee cap_{size}.log"
        print(command)
        os.system(command=command)
        if glob(flagf):
            break

if __name__ == '__main__':
    try:
        main(sys.argv)
    except KeyboardInterrupt:
        print('Experiment interrupted')
        try:
            sys.exit(0)
        except SystemExit:
            os._exit(0)