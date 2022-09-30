"""Console script."""
import argparse
import sys
import os
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
        '-m',
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
        metavar='path',
        help='Output path for the resulting trace'
    )

    args = parser.parse_args()

    return run(args)


if __name__ == "__main__":
    sys.exit(main())  # pragma: no cover

