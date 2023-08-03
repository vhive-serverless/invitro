import matplotlib.pyplot as plt
import numpy as np
import sys
from scipy import stats

distribution = sys.argv[1]
inputFile = sys.argv[2]

alpha = 0.05

f = np.loadtxt(inputFile, dtype=float)

if distribution == "uniform":
    minBoundary = 0
    maxBoundary = max(f)

    cdf = stats.uniform(loc=minBoundary, scale=maxBoundary).cdf
elif distribution == "exponential":
    maximum = max(f)
    totalDuration = float(sys.argv[3])
    for i in range(len(f)):
        f[i] = f[i] / 60_000_000 * totalDuration

    cdf = stats.expon.cdf
else:
    exit(2)  # unsupported distribution

test = stats.kstest(f, cdf)

plt.hist(f, density=True, bins=30)
plt.savefig(f"distribution_{distribution}.png")

print(test)

if test.pvalue > alpha:
    exit(0)  # the sample satisfies the distribution
else:
    exit(1)  # the sample does not satisfy the distribution
