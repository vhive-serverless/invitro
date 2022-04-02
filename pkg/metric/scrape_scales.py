import subprocess
import json
import os

if __name__ == "__main__":
    cmd = "kubectl get deploy | awk '{print $1\" \"$4}'"
    out = os.popen(cmd).read().strip().split('\n')

    results = []
    for line in out[1:]:
        deployment, scale = line.split()
        results.append({
            # Cut of the deployment suffix as each function is only deployed once.
            "deployment": '-'.join(deployment.split('-')[:-2]),
            "scale": int(scale),
        })

    print(json.dumps(results))
