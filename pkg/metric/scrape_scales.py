import json
import os

if __name__ == "__main__":
    cmd = "kubectl get podautoscalers | awk '{print $1\" \"$2\" \"$3}'"
    out = os.popen(cmd).read().strip().split('\n')

    results = []
    for line in out[1:]:
        deployment, desired_scale, actual_scale = line.split()
        results.append({
            # Cut of the deployment suffix as each function is only deployed once.
            "deployment": '-'.join(deployment.split('-')[:-1]),
            "desired_scale": int(desired_scale),
            "actual_scale": int(actual_scale),
        })

    print(json.dumps(results))
