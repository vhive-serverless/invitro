import sys
import json
import numpy as np
from statsmodels.tsa.stattools import adfuller

import warnings
warnings.filterwarnings("ignore")

if __name__ == "__main__": 
    latencies = list(map(float, sys.argv[1].strip().split('@')))
    try:
        res = adfuller(latencies, autolag='AIC')
    except ValueError as e:
        print(json.dumps("{}"))
        exit()
    

    out = {
        "statistic": res[0] if not np.isinf(res[0]) else 99999,
        "pvalue": res[1],
        "usedlag": res[2],
        "nobs": res[3],
        "critical_vals": res[4],
        "icbest": res[5] if not np.isinf(res[5]) else -99999,
    }

    print(json.dumps(out))