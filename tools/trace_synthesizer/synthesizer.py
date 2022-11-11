from util import *
import os
import pandas as pd
import numpy as np
import string
import random
import logging


def hash_generator(size):
    chars=string.ascii_lowercase + string.digits
    return ''.join(random.choice(chars) for _ in range(size))


def generate(args):
    functions = args.functions
    beginning = args.beginning
    target = args.target
    step = args.step
    duration = args.duration
    execution = args.execution
    memory = args.memory
    output_path = args.output
    logging.basicConfig(filename='synthesizer.log', level=logging.DEBUG, force=True)
    inv_df = load_data("base_traces/inv.csv")
    mem_df = load_data("base_traces/mem.csv")
    run_df = load_data("base_traces/run.csv")

    hashFunction = []
    hashOwner = []
    hashApp = []

    lenHashes = 64
    sampleCount = 1
    # needs to be > 0 to work with loader implementation on loader_unit_tests branch
    # see https://github.com/eth-easl/loader/blob/acdcde214d7a08d3603011ec5c9d28885ab3e986/pkg/generator/specification.go#L189
    for i in range(functions):
        hashFunction.append(hash_generator(lenHashes))
        hashOwner.append(hash_generator(lenHashes))
        hashApp.append(hash_generator(lenHashes))

    mem = [memory]
    mem = np.repeat(mem, len(mem_df.columns) - 4)
    run = [execution]
    run = np.repeat(run, len(run_df.columns) - 5)
    rps = [*range(beginning, target+1, step)]
    ipm = [60*x for x in rps]  # convert rps to invocations per minute
    ipm = np.repeat(ipm, duration)
    # pad with zeros to get trace that is 1440 minutes
    ipm = np.pad(ipm, (0, 1440 - len(ipm)), 'constant')

    for i in range(functions):
        memArr = [hashApp[i], hashOwner[i], hashFunction[i], sampleCount]
        memArr.extend(mem)
        mem_df.loc[len(mem_df)] = memArr
        runArr = [hashFunction[i], hashOwner[i], hashApp[i], execution, sampleCount]
        runArr.extend(run)
        run_df.loc[len(run_df)] = runArr 
        invArr = [hashApp[i], hashFunction[i], hashOwner[i]]
        invArr.extend(ipm)
        inv_df.loc[len(inv_df)] = invArr
    


    p1 = f"{output_path}/invocations.csv"
    save_data(inv_df, p1)
    logging.info(f"saved invocations to {p1}")
    p2 = f"{output_path}/memory.csv"
    save_data(mem_df, p2)
    logging.info(f"saved invocations to {p2}")
    p3 = f"{output_path}/durations.csv"
    save_data(run_df, p3)
    logging.info(f"saved invocations to {p3}")

    return inv_df, mem_df, run_df
