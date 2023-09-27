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

import matplotlib.pyplot as plt
import numpy as np
import pandas as pd
from scipy.signal import hamming, tukey


def read_minute_scale_timeline(path: str) -> pd.DataFrame:
    df = pd.read_csv(path)
    df = df.sort_values(by=["minute"])
    df = df.groupby('minute').sum().reset_index()
    return df


def plot_avg_runtime_per_min(df: pd.DataFrame, ax: plt.Axes) -> plt.Axes:
    ax.step(df["minute"] / 60, df["avgRuntime"], label="avgRuntime")
    ax.set_xlabel("time [h]")
    ax.set_ylabel("Average Runtime")
    ax.set_title("Average Runtime per minute")
    return ax


def plot_avg_cpu_per_min(df: pd.DataFrame, ax: plt.Axes) -> plt.Axes:
    ax.step(df["minute"] / 60, df["avgCpu"], label="cpu")
    ax.set_xlabel("time [h]")
    ax.set_ylabel("cpu [vCore]")
    ax.set_title("Average cpu request per minute")
    return ax


def plot_avg_memory_per_min(df: pd.DataFrame, ax: plt.Axes) -> plt.Axes:
    ax.step(df["minute"] / 60, df["avgMemory"], label="memory")
    ax.set_xlabel("time [h]")
    ax.set_ylabel("memory [mb]")
    ax.set_title("Average memory request per minute")
    return ax


def plot_fourier_transform(df: pd.DataFrame, ax: plt.Axes, start_idx: int = 1) -> plt.Axes:
    fft = abs(np.fft.rfft(df))
    freq = np.fft.rfftfreq(len(df), 1)

    ax.plot(freq[start_idx:], fft[start_idx:], label="fft")
    ax.set_xlabel("frequency [Hz]")
    ax.set_ylabel("amplitude")
    return ax


def plot_windowed_fourier_transform(df: pd.DataFrame, ax: plt.Axes, window_fnc: str, start_idx: int = 1) -> plt.Axes:
    assert window_fnc in ["hamming", "tukey"], "window_fnc must be either hamming or tukey"

    window = hamming(len(df)) if window_fnc == "hamming" else tukey(len(df))
    data = (df - np.median(df)) * window
    fft = abs(np.fft.rfft(data))
    freq = np.fft.rfftfreq(len(data), 1)

    ax.plot(freq[start_idx:], fft[start_idx:], label=f"{window_fnc} fft")
    ax.set_xlabel("frequency [Hz]")
    ax.set_ylabel("amplitude")
    return ax


def plot_fnc_cnt_per_millisecond(df: pd.DataFrame, ax: plt.Axes, x_scale: str) -> plt.Axes:
    if x_scale == "milliseconds":
        scaling = 1
    elif x_scale == "seconds":
        scaling = 1000
    elif x_scale == "minutes":
        scaling = 60 * 1000
    elif x_scale == "hours":
        scaling = 60 * 60 * 1000
    else:
        raise AssertionError(f"x_scale must be either milliseconds or seconds or minutes or hours, but is {x_scale}")

    ax.step(df["timestamp"] / scaling, df["funcCnt"], label="function count")
    ax.set_xlabel(f"time [{x_scale}]")
    ax.set_ylabel("function count")
    return ax


def plot_memory_per_millisecond(df: pd.DataFrame, ax: plt.Axes, x_scale: str) -> plt.Axes:

    if x_scale == "milliseconds":
        scaling = 1
    elif x_scale == "seconds":
        scaling = 1000
    elif x_scale == "minutes":
        scaling = 60 * 1000
    elif x_scale == "hours":
        scaling = 60 * 60 * 1000
    else:
        raise AssertionError(f"x_scale must be either milliseconds or seconds or minutes or hours, but is {x_scale}")

    ax.step(df["timestamp"] / scaling, df["memory"], label="memory")
    ax.set_xlabel(f"time [{x_scale}]")
    ax.set_ylabel("memory [mb]")
    return ax


def plot_cpu_per_millisecond(df: pd.DataFrame, ax: plt.Axes, x_scale: str) -> plt.Axes:
    if x_scale == "milliseconds":
        scaling = 1
    elif x_scale == "seconds":
        scaling = 1000
    elif x_scale == "minutes":
        scaling = 60 * 1000
    elif x_scale == "hours":
        scaling = 60 * 60 * 1000
    else:
        raise AssertionError(f"x_scale must be either milliseconds or seconds or minutes or hours, but is {x_scale}")

    ax.step(df["timestamp"] / scaling, df["cpu"], label="cpu")
    ax.set_xlabel(f"time [{x_scale}]")
    ax.set_ylabel("cpu [vCore]")
    return ax


def plot_coherence(sig1: np.ndarray, sig2: np.ndarray, **plt_kwargs) -> plt.Figure:
    fig, ax = plt.subplots(2, 1, **plt_kwargs)
    ax[0].grid(True, which='both')
    ax[0].set_xlabel('time')
    ax[0].set_ylabel('Amplitude')
    time1 = np.arange(0, len(sig1))
    time2 = np.arange(0, len(sig2))
    ax[0].plot(time1, sig1)
    ax2 = ax[0].twinx()
    ax2.plot(time2, sig2, 'r')

    coh, freq = ax[1].cohere(sig1, sig2, Fs=1)
    ax[1].set_ylabel('Coherence')
    return fig
    
