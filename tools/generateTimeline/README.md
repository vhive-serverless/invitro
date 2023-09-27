# Generate Timeline

Scripts in this directory are used to generate a total timeline of the given trace file. The scripts make use of [loader](https://github.com/eth-easl/loader) to generate memory, runtime, and cpu usages. Since this is a private repository, extra authentication is required to use the scripts.

```shell
git config --global --add url."git@github.com:".insteadOf "https://github.com/"
export GOPRIVATE=github.com/eth-easl/loader
go get github.com/eth-easl/loader
```

```shell
Usage of generateTimeline:
  -duration int
     Duration of the traces in minutes (default 1440)
  -outputFile string
     Path to output file (default "output.csv")
  -randSeed int
     Seed for the random number generator (default 42)
  -scale string
     Scale of the timeline to generate, one of [millisecond, minute] (default "millisecond")
  -tracePath string
     Path to folder where the trace is located (default "data/traces/")
```
