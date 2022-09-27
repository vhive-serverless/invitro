import sys

import numpy as np
from scipy import stats
import matplotlib.pyplot as plt

distribution = sys.argv[1]
minBoundary = float(sys.argv[2])
maxBoundary = float(sys.argv[3])
inputFile = sys.argv[4]

alpha = 0.05

f = np.loadtxt("test_data.txt", dtype=float)

if distribution == "uniform":
    cdf = stats.uniform(loc=minBoundary, scale=maxBoundary).cdf
elif distribution == "exponential":
    cdf = stats.expon.cdf
else:
    exit(300)

test = stats.kstest(f, cdf)

plt.hist(f, density=True, bins=30)
plt.savefig("distribution.png")

print(test)

if test.pvalue > alpha:
    exit(200)  # the sample satisfies the distribution
else:
    exit(400)  # the sample does not satisfy the distribution
