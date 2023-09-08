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

#  MIT License
#
#
#  Permission is hereby granted, free of charge, to any person obtaining a copy
#  of this software and associated documentation files (the "Software"), to deal
#  in the Software without restriction, including without limitation the rights
#  to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
#  copies of the Software, and to permit persons to whom the Software is
#  furnished to do so, subject to the following conditions:
#
#
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
