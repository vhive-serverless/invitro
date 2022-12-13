import numpy as np
import pandas as pd
import random
import string
import logging as log
import os
import matplotlib.pyplot as plt


def check_percentiles(percentiles, name):
    if len(percentiles) != 8:
        raise ValueError(f"{name} percentiles must be a list of 8 values")
    if sorted(percentiles) != percentiles:
        raise ValueError(f"{name} percentiles must be in ascending order")
    return


def hash_generator(size):
    chars = string.ascii_lowercase + string.digits
    return "".join(random.choice(chars) for _ in range(size))


def get_num_from_percentile(percentiles, randFloat=None):
    if randFloat is None:
        randFloat = random.random()
    if randFloat <= 0.01:
        return percentiles[0]
    elif randFloat <= 0.05:
        return random.randint(percentiles[0], percentiles[1])
    elif randFloat <= 0.25:
        return random.randint(percentiles[1], percentiles[2])
    elif randFloat <= 0.5:
        return random.randint(percentiles[2], percentiles[3])
    elif randFloat <= 0.75:
        return random.randint(percentiles[3], percentiles[4])
    elif randFloat <= 0.95:
        return random.randint(percentiles[4], percentiles[5])
    elif randFloat <= 0.99:
        return random.randint(percentiles[5], percentiles[6])
    else:
        return random.randint(percentiles[6], percentiles[7])


def build_invocation(invocations, iats, amplitude, length, duration, sync_burst_starts=False):
    randFloat = random.random()
    base_inv = get_num_from_percentile(invocations, randFloat)
    burst_amp = get_num_from_percentile(amplitude, randFloat)

    inv = np.repeat(base_inv, duration)

    iat = get_num_from_percentile(iats)
    burst_length = get_num_from_percentile(length)
    if burst_length > 0:
        slope = burst_amp / burst_length
    else:
        slope = 0

    if sync_burst_starts:
        i = 0
    else:
        i = random.randint(0, iat)
    while i < duration:
        if slope > 1:
            for j in range(burst_length):
                if i + j < duration:
                    inv[i + j] += int(slope * (j + 1))
        else:
            inv[i] = inv[i] + burst_amp

        i += iat + 1

    inv = np.pad(inv, (0, 1440 - len(inv)), "constant")
    return inv


def synthesize(args):
    functions = args.functions
    duration = min(args.duration, 1440)
    invocations = args.invocations
    iat = args.iat
    amplitude = args.amplitude
    length = args.length
    memory = args.memory
    execution = args.execution
    sync_burst_starts = args.sync_burst_starts

    dir_path = os.path.dirname(os.path.realpath(__file__))

    inv_df = pd.read_csv(f"{dir_path}/../trace_synthesizer/base_traces/inv.csv")
    mem_df = pd.read_csv(f"{dir_path}/../trace_synthesizer/base_traces/mem.csv")
    run_df = pd.read_csv(f"{dir_path}/../trace_synthesizer/base_traces/run.csv")

    check_percentiles(invocations, "invocations")
    check_percentiles(iat, "burst iat")
    check_percentiles(amplitude, "burst amplitude")
    check_percentiles(length, "burst length")

    hashFunction = []
    hashOwner = []
    hashApp = []

    lenHashes = 64
    sampleCount = 1

    for i in range(functions):
        hashFunction.append(hash_generator(lenHashes))
        hashOwner.append(hash_generator(lenHashes))
        hashApp.append(hash_generator(lenHashes))

    mem = [memory]
    mem = np.repeat(mem, len(mem_df.columns) - 4)
    run = [execution]
    run = np.repeat(run, len(run_df.columns) - 5)

    for i in range(functions):
        memArr = [hashApp[i], hashOwner[i], hashFunction[i], sampleCount]
        memArr.extend(mem)
        mem_df.loc[len(mem_df)] = memArr
        runArr = [hashFunction[i], hashOwner[i], hashApp[i], execution, sampleCount]
        runArr.extend(run)
        run_df.loc[len(run_df)] = runArr
        invArr = [hashApp[i], hashFunction[i], hashOwner[i]]
        invArr.extend(build_invocation(invocations, iat, amplitude, length, duration, sync_burst_starts))
        inv_df.loc[len(inv_df)] = invArr

    p1 = f"{args.output}/invocations.csv"
    inv_df.to_csv(p1, index=False)
    log.info(f"saved invocations to {p1}")
    p2 = f"{args.output}/memory.csv"
    mem_df.to_csv(p2, index=False)
    log.info(f"saved invocations to {p2}")
    p3 = f"{args.output}/durations.csv"
    run_df.to_csv(p3, index=False)
    log.info(f"saved invocations to {p3}")

    no_invs = inv_df[[str(i) for i in range(1, duration + 1)]].sum(axis=0).to_numpy()
    fig, ax = plt.subplots()
    ax.plot(no_invs)
    ax.set_xlabel("Time (minutes)")
    ax.set_ylabel("Total Invocations")
    ax.set_ylim(0, 1.1 * max(no_invs))
    fig.savefig(f"{args.output}/invocations.png")

    med_invs = inv_df.median(axis=1).to_numpy()
    fig, ax = plt.subplots(1, 2)
    box = ax[0].violinplot(med_invs)
    ax[0].set_ylabel("Invocations")
    ax[1].hist(med_invs, bins=100)
    ax[1].set_xlabel("Invocations")
    fig.suptitle(f"Median Invocations Per Function")
    fig.savefig(f"{args.output}/invocations_box.png")
    plt.close()
