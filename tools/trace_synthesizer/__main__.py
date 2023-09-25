"""Console script."""
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

import argparse
import os
import sys

from synthesizer import generate


def run(args):
    if not os.path.exists(args.output):
        try:
            os.makedirs(args.output)
        except OSError as e:
            raise RuntimeError(f"Failed to create the output folder: {e}")

    if args.cmd == 'generate':
        inv_df, mem_df, run_df = generate(args)

    return


def main():
    parser = argparse.ArgumentParser()
    subparser = parser.add_subparsers(dest="cmd")

    gen_parser = subparser.add_parser('generate')

    gen_parser.add_argument(
        '-f',
        '--functions',
        required=False,
        type=int,
        default=1,
        metavar='integer',
        help='Number of functions in the trace'
    )

    gen_parser.add_argument(
        '-b',
        '--beginning',
        required=True,
        type=int,
        metavar='integer',
        help='Starting RPS value'
    )

    gen_parser.add_argument(
        '-t',
        '--target',
        required=True,
        type=int,
        metavar='integer',
        help='Maximum'
    )

    gen_parser.add_argument(
        '-s',
        '--step',
        required=True,
        type=int,
        metavar='integer',
        help='Step size'
    )

    gen_parser.add_argument(
        '-dur',
        '--duration',
        required=True,
        type=int,
        metavar='integer',
        help='Duration of each RPS slot in minutes'
    )

    gen_parser.add_argument(
        '-e',
        '--execution',
        required=False,
        type=int,
        default=1000,
        metavar='integer',
        help='Execution time of the functions in ms'
    )

    gen_parser.add_argument(
        '-mem',
        '--memory',
        required=False,
        type=int,
        default=120,
        metavar='integer',
        help='Memory usage of the functions in MB'
    )

    gen_parser.add_argument(
        '-o',
        '--output',
        required=True,
        type=str,
        metavar='path',
        help='Output path for the resulting trace'
    )

    gen_parser.add_argument(
        '-m',
        '--mode',
        required=True,
        type=int,
        metavar='integer',
        help='Normal [0]; RPS sweep [1]; Burst [2]'
    )

    args = parser.parse_args()

    return run(args)


if __name__ == "__main__":
    sys.exit(main())  # pragma: no cover
