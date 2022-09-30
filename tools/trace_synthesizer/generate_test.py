import pandas as pd
import unittest

from synthesizer import generate


class Empty:
    pass


def test_generate():
    args = Empty()
    args.functions = 2
    args.beginning = 10
    args.target = 20
    args.step = 5
    args.duration = 3
    args.output = 'test_output'
    args.execution = 700
    args.memory = 200
    inv_df, mem_df, run_df = generate(args)

    assert len(inv_df.index) == args.functions
    # number of functions has to equal number of rows
    sumInvocations = 0
    for i in range(3, 3*args.duration+3):
        sumInvocations += inv_df.iloc[:, i].sum()
    expectedSum = (600 + 900 + 1200) * 3 * 2
    # 3 min per slot, 2 functions
    assert sumInvocations == expectedSum
    

if __name__ == '__main__':
    test_generate()
