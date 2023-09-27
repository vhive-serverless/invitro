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
import logging as log
import os
import sys
import pandas as pd

# from sampler.plot import plot_cdf
from sampler.preprocess import parse_trace_files
from sampler.sample import generate_samples

log.basicConfig(format='%(levelname)s:%(message)s', level=log.INFO)

def run(args):
    log.info('Loading trace data...')

    if not os.path.exists(args.source_trace if 'source_trace' in args else args.trace):
        raise RuntimeError('Input trace folder does not exist')

    if not os.path.exists(args.output) and args.cmd != 'plot':
        try:
            os.makedirs(args.output)
        except OSError as e:
            raise RuntimeError(f"Failed to create the output folder: {e}")

    if args.cmd == 'preprocess':
        startTime = args.start
        startTime = startTime.split(":")
        startingDay = int(startTime[0])
        hours = int(startTime[1])
        minutes = int(startTime[2])
        duration = int(args.duration)
        assert hours * 60 + minutes + duration <= 1440, "Time slice cannot span multiple days"
        startingDay = startingDay + 1  # Because first day is day 0, but first file is named d01
        assert startingDay <= 12, "There is only complete data for days 1 through 12"
        inv_df, run_df, mem_df = parse_trace_files(args.trace, starting_day=startingDay, hours=hours,
                                                   minutes=minutes, dur=duration)

        inv_df.to_csv(f"{args.output}/invocations.csv", index=False)
        mem_df.to_csv(f"{args.output}/memory.csv", index=False)
        run_df.to_csv(f"{args.output}/durations.csv", index=False)

        return

    inv_df = pd.read_csv(f"{args.source_trace}/invocations.csv")
    mem_df = pd.read_csv(f"{args.source_trace}/memory.csv")
    run_df = pd.read_csv(f"{args.source_trace}/durations.csv")

    if args.original_trace == None:
        args.original_trace = args.source_trace
    inv_df_orig = pd.read_csv(f"{args.original_trace}/invocations.csv")
    mem_df_orig = pd.read_csv(f"{args.original_trace}/memory.csv")
    run_df_orig = pd.read_csv(f"{args.original_trace}/durations.csv")

    if args.cmd == 'sample':
        generate_samples(inv_df=inv_df, mem_df=mem_df, run_df=run_df,
                         inv_df_orig=inv_df_orig, mem_df_orig=mem_df_orig, run_df_orig=run_df_orig,
                         min_size=args.min_size, step=args.step_size, max_size=args.max_size,
                         trial_num=args.trial_num,
                         out_path=args.output)

    if args.cmd == 'plot':
        log.fatal(f"Plotting is currently broken (Issue: https://github.com/eth-easl/sampler/issues/77)")
        return

        # sample = pd.read_csv(f"{args.source_trace}/{args.sample}")
        # plot_cdf(kind=args.kind, output=args.output, inv_df=inv_df, run_df=run_df, mem_df=mem_df, sample=sample)

    return


def main():
    parser = argparse.ArgumentParser()
    subparser = parser.add_subparsers(dest="cmd")

    sample_parser = subparser.add_parser('sample')

    sample_parser.add_argument(
        '-t',
        '--source_trace',
        required=True,
        metavar='path',
        help='Path to trace to draw samples from'
    )

    sample_parser.add_argument(
        '-orig',
        '--original_trace',
        metavar='path',
        default=None,
        help='Path to the Azure (or other original) trace files, required to maximize the derived sample\'s representativity (WD from the original trace)'
    )

    sample_parser.add_argument(
        '-o',
        '--output',
        required=True,
        metavar='path',
        help='Output path for the resulting samples'
    )

    sample_parser.add_argument(
        '-min',
        '--min-size',
        required=False,
        type=int,
        default=1000,
        metavar='integer',
        help='Minimum sample size (#functions).'
    )

    sample_parser.add_argument(
        '-st',
        '--step-size',
        required=False,
        type=int,
        default=1000,
        metavar='integer',
        help='Step (#functions) in sample size during sampling.',
    )

    sample_parser.add_argument(
        '-max',
        '--max-size',
        required=False,
        type=int,
        default=20000,
        metavar='integer',
        help='Maximum sample size (#functions).'
    )

    sample_parser.add_argument(
        '-tr',
        '--trial-num',
        required=False,
        type=int,
        default=16,
        metavar='integer',
        help='Number of sampling trials for each sample size.'
    )

    ####################################################
    plot_parser = subparser.add_parser('plot')

    plot_parser.add_argument(
        '-t',
        '--trace',
        required=True,
        metavar='path',
        help='Path to the trace'
    )

    plot_parser.add_argument(
        '-k',
        '--kind',
        required=True,
        metavar='[invocations, durations, memory]',
        help='Generate CDF for a single dimension'
    )

    plot_parser.add_argument(
        '-s',
        '--sample',
        required=True,
        metavar='path',
        help='Path to the sample to be visualised'
    )

    plot_parser.add_argument(
        '-o',
        '--output',
        required=True,
        metavar='path',
        help='Output path for the output figure'
    )

    ####################################################
    pre_parser = subparser.add_parser('preprocess')

    pre_parser.add_argument(
        '-t',
        '--trace',
        metavar='path',
        default='data/azure',
        help='Path to the Azure trace files'
    )

    pre_parser.add_argument(
        '-o',
        '--output',
        required=True,
        metavar='path',
        help='Output path for the preprocessed traces'
    )

    pre_parser.add_argument(
        '-s',
        '--start',
        required=True,
        metavar='start',
        help='Time in dd:hh:mm format, at which the excerpt of the postprocessed trace should begin, first day is day 0'
    )

    pre_parser.add_argument(
        '-dur',
        '--duration',
        required=True,
        metavar='duration',
        help='Duration in minutes of the excerpt extracted from the postprocessed trace'
    )

    ####################################################

    args = parser.parse_args()

    return run(args)


if __name__ == "__main__":
    sys.exit(main())  # pragma: no cover
