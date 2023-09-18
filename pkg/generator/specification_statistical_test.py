#  MIT License
#
#  Copyright (c) 2023 EASL and the vHive community
#
#  Permission is hereby granted, free of charge, to any person obtaining a copy
#  of this software and associated documentation files (the "Software"), to deal
#  in the Software without restriction, including without limitation the rights
#  to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
#  copies of the Software, and to permit persons to whom the Software is
#  furnished to do so, subject to the following conditions:
#
#  The above copyright notice and this permission notice shall be included in all
#  copies or substantial portions of the Software.
#
#  THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
#  IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
#  FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
#  AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
#  LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
#  OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
#  SOFTWARE.

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
