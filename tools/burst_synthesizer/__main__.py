"""Console script."""
import argparse
import sys
import os
import logging as log
from synthesizer import synthesize


def main():
    log.basicConfig(level=log.INFO, format="(%(asctime)s) Burst synthesizer -- [%(levelname)s] %(message)s")
    args = parse_args()
    if not os.path.exists(args.output):
        try:
            os.makedirs(args.output)
        except OSError as e:
            raise RuntimeError(f"Failed to create the output folder: {e}")

    synthesize(args)

    return


def parse_args():
    parser = argparse.ArgumentParser()

    parser.add_argument(
        "-f",
        "--functions",
        required=False,
        type=int,
        default=1,
        metavar="integer",
        help="Number of functions in the trace",
    )

    parser.add_argument(
        "-dur", "--duration", required=True, type=int, metavar="integer", help="Duration of synthetic trace in minutes"
    )

    parser.add_argument(
        "-i",
        "--invocations",
        nargs=8,
        required=True,
        type=int,
        metavar="percentile",
        help="Number of invocations for each function, enter the 1, 5, 25, 50, 75, 95, 99, 100 percentiles",
    )

    parser.add_argument(
        "-iat",
        "--iat",
        nargs=8,
        required=True,
        type=int,
        metavar="percentile",
        help="Interarrival time of bursts, enter the 1, 5, 25, 50, 75, 95, 99, 100 percentiles",
    )

    parser.add_argument(
        "-a",
        "--amplitude",
        nargs=8,
        required=True,
        type=int,
        metavar="percentile",
        help="Amplitude of bursts, enter the 1, 5, 25, 50, 75, 95, 99, 100 percentiles",
    )

    parser.add_argument(
        "-l",
        "--length",
        nargs=8,
        required=True,
        type=int,
        metavar="percentile",
        help="Length of bursts, enter the 1, 5, 25, 50, 75, 95, 99, 100 percentiles",
    )

    parser.add_argument(
        "-e",
        "--execution",
        required=False,
        type=int,
        default=1000,
        metavar="integer",
        help="Execution time of the functions in ms",
    )

    parser.add_argument(
        "-m",
        "--memory",
        required=False,
        type=int,
        default=120,
        metavar="integer",
        help="Memory usage of the functions in MB",
    )

    parser.add_argument("-o", "--output", required=True, metavar="path", help="Output path for the resulting trace")

    args = parser.parse_args()

    return args


if __name__ == "__main__":
    sys.exit(main())  # pragma: no cover
