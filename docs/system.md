# `loader` System Overview 

```bash
.
├── Dockerfile.trace.*container*  # Image for benchmark functions
├── Makefile    # Relevant cmds for interacting with loader
├── cmd
│   ├── load.go     # Main program entry
│   └── options
│       └── warmup.go   # Warm-up code
├── config
│   ├── functions.json  # Example function config from vhive (not used)
│   ├── k8s_scheduler_patch.yaml    # Patch for scheduler to be scraped
│   ├── kn_configmap_warmup_init_patch.yaml     # Patch for initializing warm-up
│   ├── kn_configmap_warmup_reset_patch.yaml    # Patch for resetting warm-up (global keys)
│   ├── kn_serving_core.yaml    # Backup of control plane quotas for experiments (in the `hy` branch of vhive )
│   ├── kpa_reset_patch.yaml    # Patch for resetting warm-up (local KPA)
│   ├── kubeadm_init.yaml  # Restart k8s upon boosting
│   ├── kubeadm_init_large.yaml
│   ├── metrics_cluster_role.yaml  # Grant security for prometheus stack
│   ├── metrics_server_components.yaml  # All components needed for metric server to work
│   ├── otel_kn.yaml  # OpenT Not used.
│   ├── prometh_kn.yaml  # Knative config in promethues
│   ├── prometh_stack_values.yaml
│   ├── prometh_values_kn.yaml
│   ├── requirements.txt  # Dependencies for metric collectors (py)
│   ├── requirements_dev.txt
│   └── vhive  # Backup yamls integrated in vhive `hy` branch
│       ├── loader_istio_controller.yaml
│       └── loader_serving_core.yaml
├── data
│   ├── coldstarts  # Cold start experiment data
│   ├── logs
│   ├── out
│   ├── samples  # Different kinds of trace samples
│   └── traces   # Authantic sample traces used in experiments
│
├── docs
├── go.mod
├── go.sum
│
├── pkg
│   ├── function
│   │   ├── deploy.go  # Deploys functions
│   │   ├── deploy.sh  # Pipes funciton definitions based on the trace
│   │   ├── invoke.go  # Invokes individual functions
│   │   ├── registry.go  # Gauges memory load 
│   │   └── rpcpool.go  # Pools RPC connections (not used anymore)
│   ├── generate
│   │   ├── atom.go  # Common functions and parameters
│   │   ├── burst_load.go  # Generates bursts
│   │   ├── coldstart_load.go  # Generates cold-start load
│   │   ├── rps_load.go  # Generates synthetic RPS sweeps
│   │   └── trace_load.go  # Generates real workloads from traces
│   ├── metric
│   │   ├── collect.go  # Collects and exports all metrics
│   │   ├── record.go  # Defines data models for metric records
│   │   ├── run_adf.py  # Invokes the ADF stationarity test
│   │   ├── scale_registry.go  # Gauges function scales
│   │   ├── scrape_infra.py  # Scrapes infra related metrics, e.g., CPU, mem from **k8s**
│   │   ├── scrape_kn.py  # Scrapes relevant metrics from **kn**
│   │   └── scrape_scales.py  # Collects scales for **each** function
│   ├── test
│   │   ├── collect_test.go
│   │   ├── generate_test.go
│   │   ├── load_registry_test.go
│   │   ├── parse_test.go
│   │   ├── run_adf.py
│   │   ├── scale_registry_test.go
│   │   ├── test_inv.csv
│   │   ├── util_test.go
│   │   └── warmup_test.go
│   ├── trace
│   │   ├── model.go  # Defines the data model of the trace records
│   │   ├── parse.go  # Reads and preprocesses trace
│   │   └── profile.go  # Profiles function concurrencies
│   └── util.go
├── scripts
│   ├── experiments
│   │   ├── drive_trace_mode.py  # Trace mode script
│   │   ├── feed_prior_works.py  # Feeds the loader with workloads from prior workds
│   │   ├── feed_same_size.py  # Loads many traces of the same size for convergence experiments
│   │   ├── run_burst_mode.sh  # Burst mode script
│   │   ├── run_coldstart_mode.sh  # Cold-start mode script
│   │   ├── run_convergence.sh  # Wrapper of the convergence experiment
│   │   ├── run_prior_works.sh  # Wrapper for comparing prior workds
│   │   ├── run_rps_mode.sh  # RPS mode script
│   │   └── run_trace_mode.sh  # Wrapper for trace mode
│   ├── isolation
│   │   ├── cgexec.sh  # Runs loader with cgroup
│   │   └── define_cgroup.sh  # Defines cgroup SPECIFICALLY for **d430 machine on cloud lab**
│   ├── metrics
│   │   ├── get_loader_cpu_pct.sh  # Scrapes node CPU in percentages
│   │   ├── get_node_stats_abs.sh  # Scrapes CPU and memory in absolute values
│   │   └── get_node_stats_percent.sh  # Scrapes node infra metrics in percentages
│   ├── plot
│   ├── setup
│   │   ├── create_multinode_container.sh
│   │   ├── create_multinode_container_large.sh
│   │   ├── create_multinode_firecracker.sh
│   │   ├── create_singlenode_container.sh
│   │   ├── expose_infra_metrics.sh
│   │   ├── fix_rounting.sh
│   │   ├── patch_init_scale.sh
│   │   ├── setup.cfg
│   │   ├── setup_trace_visualizer.sh
│   │   ├── stretch_cluster_capacity.sh
│   │   ├── taint.sh
│   │   └── turbo_boost.sh
│   ├── util  # See README
│   │   ├── check_node_capacity.sh
│   │   ├── check_pod_count.sh
│   │   ├── get_pod_cidr.sh
│   │   ├── log_kn_status.sh
│   │   └── set_function_scale.sh
│   └── warmup
│       ├── livepatch_kpa.sh
│       ├── patch_activator.sh
│       └── reset_kn_global.sh
├── server
│   ├── benchmark  # Benchmarks function timing (see README)
│   │   ├── Dockerfile.timing
│   │   ├── drive_benchmark.py
│   │   ├── go.mod
│   │   ├── go.sum
│   │   ├── timing.go
│   │   ├── timing.yaml
│   │   └── timing_test.go
│   ├── faas.pb.go
│   ├── faas.proto
│   ├── faas_grpc.pb.go
│   ├── faas_pb2.py
│   ├── faas_pb2_grpc.py
│   ├── helloworld  # Unused 
│   │   ├── Dockerfile
│   │   ├── go.mod
│   │   └── helloworld.go
│   ├── test  # Tests whether the nodes are properly saturated with specific number of containers
│   │   ├── Dockerfile.testing
│   │   ├── go.mod
│   │   ├── test_func.go
│   │   └── test_func.yaml
│   ├── timed  # Unused 
│   │   └── timed.go
│   ├── trace-func-go  # Used function
│   │   ├── trace_func.go
│   │   └── trace_func_test.go
│   ├── trace-func-py  # Unused 
│   │   ├── Dockerfile
│   │   ├── faas.proto
│   │   ├── faas_pb2.py
│   │   ├── faas_pb2_grpc.py
│   │   └── trace_func.py
│   └── wimpy  # Demo of implementation pitfalls
│       └── wimpy.go
├── tools
│   ├── bin
│   │   ├── grpcurl  # Unused 
│   │   ├── invoker  # Stand-alone invoker binary for testing (needs to recompiled if protobuf changes)
│   │   └── promql  # Executes prometheus queries on cmd directly (very useful tool, old version since new ones are not compatible)
│   └── invoker  # For the stand-alone invoker binary
│       ├── go.mod
│       └── invoker.go
└── workloads
    ├── container  # Container function definitions
    │   ├── trace_func_go.yaml
    │   ├── trace_func_py.yaml
    │   └── wimpy.yaml
    ├── firecracker  # uVM function definitions (haven't been tested for a while)
    │   ├── timed.yaml
    │   └── trace_func_go.yaml
    └── other  # Unused 
        ├── helloworld.yaml
        └── producer.yaml
```