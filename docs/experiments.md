# In-Vitro Clusters Experiments

## Trace Sweep

### Setup:
* Cluster setup: 3 nodes (1 master, 2 workers) `d430` on cloudlab
* Loader branch: `hy/generate`
* vHive branch: `hy`
* Traces:
    - [Roll-up samples](https://github.com/eth-easl/loader/tree/hy/generate/data/traces/10-1k) (unit sample size: 10 functions, largest sample size: 1K)
    - [Random samples](https://github.com/eth-easl/loader/tree/hy/generate/data/samples/random) (specs same as above)

### Steps

1. Set up >=2 clusters with the above specs, using commands listed [here](https://github.com/eth-easl/loader/tree/hy/generate#create-a-cluster).
2. Use one set of clusters for random samples and the rest for roll-up samples. 
3. For roll-up samples, use the [command](https://github.com/eth-easl/loader/tree/hy/generate#experiment) to trigger the scripts for trace mode. **NB:** Please do NOT run the python scripts wrapped by shell scripts directly, since we need to use `cgroup` to start the parent process.
4. For random samples, first replace all `"traces/10-1k/"` in `scripts/experiments/drive_trace_mode.py`, then comment out line 36 (s.t. the experiments continue even the cluster is overloaded, which is expected for random samples). Next, run the same command in step 3 to kick off the experiments.
5. Repeat the experiments for both random and roll-up sampels for >= 10 times each. **NB:** Please replace the `out/` directory after each run, otherwise, it will be overwritten by `loader`. (E.g., you can automate this by simply `mv` it a different name and `mkdir` a new one). 
6. Fetch the results to your PC or a server into seperate directories, e.g., `data/sweep/rollup or random<i>`.
7. Use `scripts/plot/trace_sweep.py` for plotting (change the data directories therein respectively).


