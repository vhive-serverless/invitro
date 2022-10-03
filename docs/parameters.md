# System Parameters

## Load Constants
```go
/** 
    pkg/atom.go 
*/

// The stationary p-value for the ADF test that warns users if the cluster hasn't been warmed up
// after predefined period.
STATIONARY_P_VALUE = 0.05
// K8s default eviction duration, after which all decisions made before should either be executed
// or failed (and cleaned).
PROFILING_DURATION_MINUTES = 5
// Ten-minute warmup for unifying the starting time when the experiments consists of multiple runs.
WARMUP_DURATION_MINUTES = 10
// The fraction of RETURNED failures to the total invocations fired. This threshold is a patent overestimation
// and it's here to stop the sweeping when the cluster is no longer functioning.
OVERFLOAD_THRESHOLD = 0.3
// The number of times allowed for the measured failure rate to surpass the `OVERFLOAD_THRESHOLD`.
// It's here to avoid "early stopping" so that we make sure sufficient load has been imposed on the system.
OVERFLOAD_TOLERANCE = 2
// The compulsory timeout after which the loader will no longer await the goroutines that haven't returned,
// and move on to the next generation round. We need it because some functions may end up in nowhere and never return.
// By default, the wait-group will halt forever in that case.
FORCE_TIMEOUT_MINUTE = 15
// The portion of measurements we take in the RPS mode. The first 20% serves as a step-wise warm-up, and
// we only take the last 80% of the measurements.
RPS_WARMUP_FRACTION = 0.2
// The maximum step size in the early stage of the RPS mode -- we shouldn't take too large a RPS step before reaching
// ~100RPS in order to ensure sufficient number of measurements for lower variance (smaller the RPS, the less total data points).
MAX_RPS_STARTUP_STEP = 5
```

## Benchmark Function Configurations
```yaml
## workloads/container/*.yaml

autoscaling.knative.dev/initial-scale: "0"  # Should start from 0, otherwise we can't deploy more functions than the node physically permits.
autoscaling.knative.dev/min-scale: "0"  # This parameter only has a per-revision key, so it's necessary to have here in case of the warmup messes up.
autoscaling.knative.dev/target-utilization-percentage: "100"  # Enforce container concurrency at any time.
autoscaling.knative.dev/target-burst-capacity: "-1"  # Put activator always in the path explicitly.
autoscaling.knative.dev/max-scale: "200"  # Maximum instances limit of Azure.
```