# Trace Synthesizer

## Usage

### Generate a synthetic trace

The trace synthesizer is a replacement for the decommissioned RPS mode of the loader.
The user can define a starting RPS, step size and the target RPS of the last slot.
This then gets converted into a csv file containing the invocations per minute for the synthetic trace,
which can then be run in the trace mode of the Loader.
Further the user can specify the execution time of the function(s), as well as their memory footprint.
These are used to generate the memory and durations csv files. Like in the decommissioned RPS mode,
the user can also specify the number of functions, which will simply be functions with different names,
which then use different instances. All functions have the same execution time, memory footprint and RPS.  

From within the `trace_synthesizer` folder, use:


```console
python3 . generate -h

usage: . generate [-h] [-f integer] -b integer -t integer -s integer -dur integer [-e integer] [-m integer] -o path

optional arguments:
  -h, --help            show this help message and exit
  -f integer, --functions integer
                        Number of functions in the trace
  -b integer, --beginning integer
                        Starting RPS value
  -t integer, --target integer
                        Maximum RPS value
  -s integer, --step integer
                        Step size
  -dur integer, --duration integer
                        Duration of each RPS slot in minutes
  -e integer, --execution integer
                        Execution time of the functions in ms
  -mem integer, --memory integer
                        Memory usage of the functions in MB
  -o path, --output path
                        Output path for the resulting trace
  -m integer, --mode integer
                        Normal [0]; RPS sweep [1]; Burst [2]
```


### Example

```bash
# Normal mode is to invoke all functions at increasing rate
# E.g. For 2 functions, starting at 10 RPS and maximum of 20 RPS with step = 5, each function will be invoked 600 times at 1st, 2nd and 3rd minute; 900 times at 4th, 5th and 6th minute; 1200 times at 7th, 8th and 9th minute
# RPS is multiplied by 60 by default, to change this, edit `ipm = [60 * x for x in rps]` in `synthesizer.py`
# `-dur` determines how long each RPS should continue for (in minutes) before increasing to the next RPS step
python3 . generate -f 2 -b 10 -t 20 -s 5 -dur 3 -e 500 -mem 350 -o example_normal -m 0

# RPS Sweep mode is to spread the invocations across all functions evenly
# E.g. Across the default 10 minute cycle (i.e. `padding=10`) and for 2 functions, functions will be invoked one by one every 5 minutes
# E.g. Across a 15 minute cycle (i.e. `padding=15`) and for 5 functions, functions will be invoked one by one every 3 minutes

# For RPS Sweep mode, ensure that -dur value matches the number of minutes in `base_traces\inv.csv`
# If an error occurs, you may edit `padding=10` in `synthesizer.py` such that duration is divisible by padding (e.g. duration=1440 is divisible by padding=10)
python3 . generate -f 2 -b 10 -t 20 -s 5 -dur 1440 -e 500 -mem 350 -o example_sweep -m 1

# Burst mode is to invoke all the functions once at the same time
# E.g. Across the default 3 minute cycle (i.e. `p = [1, 0, 0]`) and for 2 functions, all 2 functions will be invoked once every 3 minute
# E.g. Across a 5 minute cycle (i.e. `p = [1, 0, 0, 0, 0]`) and for 5 functions, all 5 functions will be invoked once every 5 minute

# For Burst mode, ensure that -dur value matches the number of minutes in `base_traces\inv.csv`
# If an error occurs, you may edit `p = [1, 0, 0]` in `synthesizer.py` such that duration is divisible by length of `p` (e.g. duration=1440 is divisible by len(p)=3)
python3 . generate -f 2 -b 10 -t 20 -s 5 -dur 1440 -e 500 -mem 350 -o example_burst -m 2
```

