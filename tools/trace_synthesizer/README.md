# Trace Synthesizer

## Usage

### Generate a synthetic trace

The trace synthesizer is a replacement for the decommissioned RPS mode of the loader. The user can define a starting RPS, step size and the target RPS of the last slot. This then gets converted into a csv file containing the invocations per minute for the synthetic trace, which can then be run in the trace mode of the Loader. Further the user can specify the execution time of the function(s), as well as their memory footprint. These are used to generate the memory and durations csv files. Like in the decommissioned RPS mode, the user can also specify the number of functions, which will simply be functions with different names, which then use different instances. All functions have the same execution time, memory footprint and RPS.   
From within the trace_synthesizer folder, use:


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
                        Target RPS value that is achieved in the last RPS slot
  -s integer, --step integer
                        Step size
  -dur integer, --duration integer
                        Duration of each RPS slot in minutes
  -e integer, --execution integer
                        Execution time of the functions in ms
  -m integer, --memory integer
                        Memory usage of the functions in MB
  -o path, --output path
                        Output path for the resulting trace
```



### Example

```bash
python3 trace_synthesizer generate -f 2 -b 10 -t 20 -s 5 -dur 3 -e 500 -m 350 -o example 
```

