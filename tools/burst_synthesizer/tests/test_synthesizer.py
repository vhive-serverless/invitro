import os
import pandas as pd
from ..synthesizer import build_invocation, synthesize


def test_build_invocation():
    invocations = [0, 0, 0, 0, 0, 0, 0, 0]
    iats = [2, 2, 2, 2, 2, 2, 2, 2]
    amplitude = [1, 1, 1, 1, 1, 1, 1, 1]
    length = [1, 1, 1, 1, 1, 1, 1, 1]
    duration = 10
    sync_burst_starts = False
    inv = build_invocation(invocations, iats, amplitude, length, duration, sync_burst_starts)
    assert len(inv) == 1440

    assert all([a == b for a, b in zip(inv[duration:], ([0] * (1440 - duration)))])


def test_synthesize():
    args = type("args", (object,), {})()
    args.functions = 10
    args.duration = 10
    args.invocations = [0, 0, 0, 0, 0, 0, 0, 0]
    args.iat = [2, 2, 2, 2, 2, 2, 2, 2]
    args.amplitude = [1, 1, 1, 1, 1, 1, 1, 1]
    args.length = [1, 1, 1, 1, 1, 1, 1, 1]
    args.execution = 700
    args.memory = 200
    args.sync_burst_starts = False
    args.output = "./test_output"
    if not os.path.exists(args.output):
        os.makedirs(args.output)

    synthesize(args)
    inv_df = pd.read_csv(f"{args.output}/invocations.csv")
    mem_df = pd.read_csv(f"{args.output}/memory.csv")
    run_df = pd.read_csv(f"{args.output}/durations.csv")

    assert len(inv_df.index) == args.functions
    assert inv_df["HashFunction"].equals(mem_df["HashFunction"])
    assert inv_df["HashFunction"].equals(run_df["HashFunction"])
