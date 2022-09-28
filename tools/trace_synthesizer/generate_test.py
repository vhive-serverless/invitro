import pandas as pd
import unittest

# from trace_synthesizer.synthesizer import generate
from synthesizer import generate
# import synthesizer 

def test_generate():
    durmin = 3
    f = 2
    save = False
    inv_df, mem_df, run_df = generate(f, 10, 20, 5, durmin, 700, 200, 'examples', save)

    assert len(inv_df.index) == f
    # number of functions has to equal number of rows
    sumInvocations = 0
    for i in range(3, 3*durmin+3):
        sumInvocations += inv_df.iloc[:,i].sum()
    expectedSum = (600 + 900 + 1200) * 3 * 2
    # 3 min per slot, 2 functions
    assert sumInvocations == expectedSum
    # print(sumInvocations)
    print(0)
    

if __name__ == '__main__':
    test_generate()
