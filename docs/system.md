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
│   ├── kubeadm_init.yaml
│   ├── kubeadm_init_large.yaml
│   ├── metrics_cluster_role.yaml
│   ├── metrics_server_components.yaml
│   ├── otel_kn.yaml
│   ├── prometh_kn.yaml
│   ├── prometh_stack_values.yaml
│   ├── prometh_values_kn.yaml
│   ├── requirements.txt
│   ├── requirements_dev.txt
│   └── vhive
│       ├── loader_istio_controller.yaml
│       └── loader_serving_core.yaml
├── data
│   ├── coldstarts
│   ├── logs
│   ├── out
│   ├── samples
│   └── traces
│
├── docs
│   ├── experiments.md
│   ├── parameters.md
│   └── system.md
├── go.mod
├── go.sum
├── pkg
│   ├── function
│   │   ├── deploy.go
│   │   ├── deploy.sh
│   │   ├── invoke.go
│   │   ├── registry.go
│   │   └── rpcpool.go
│   ├── generate
│   │   ├── atom.go
│   │   ├── burst_load.go
│   │   ├── coldstart_load.go
│   │   ├── rps_load.go
│   │   └── trace_load.go
│   ├── metric
│   │   ├── collect.go
│   │   ├── record.go
│   │   ├── run_adf.py
│   │   ├── scale_registry.go
│   │   ├── scrape_infra.py
│   │   ├── scrape_kn.py
│   │   └── scrape_scales.py
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
│   │   ├── model.go
│   │   ├── parse.go
│   │   └── profile.go
│   └── util.go
├── scripts
│   ├── experiments
│   │   ├── drive_trace_mode.py
│   │   ├── feed_prior_works.py
│   │   ├── feed_same_size.py
│   │   ├── run_burst_mode.sh
│   │   ├── run_coldstart_mode.sh
│   │   ├── run_convergence.sh
│   │   ├── run_prior_works.sh
│   │   ├── run_rps_mode.sh
│   │   └── run_trace_mode.sh
│   ├── isolation
│   │   ├── cgexec.sh
│   │   └── define_cgroup.sh
│   ├── metrics
│   │   ├── get_loader_cpu_pct.sh
│   │   ├── get_node_stats_abs.sh
│   │   └── get_node_stats_percent.sh
│   ├── plot
│   │   ├── converge.py
│   │   ├── load_comp.py
│   │   ├── trace_sweep.py
│   │   ├── variation.py
│   │   └── workload_models.py
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
│   ├── util
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
│   ├── benchmark
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
│   ├── helloworld
│   │   ├── Dockerfile
│   │   ├── go.mod
│   │   └── helloworld.go
│   ├── test
│   │   ├── Dockerfile.testing
│   │   ├── go.mod
│   │   ├── test_func.go
│   │   └── test_func.yaml
│   ├── timed
│   │   └── timed.go
│   ├── trace-func-go
│   │   ├── trace_func.go
│   │   └── trace_func_test.go
│   ├── trace-func-py
│   │   ├── Dockerfile
│   │   ├── faas.proto
│   │   ├── faas_pb2.py
│   │   ├── faas_pb2_grpc.py
│   │   └── trace_func.py
│   └── wimpy
│       └── wimpy.go
├── tools
│   ├── bin
│   │   ├── grpcurl
│   │   ├── invoker
│   │   └── promql
│   └── invoker
│       ├── go.mod
│       └── invoker.go
└── workloads
    ├── container
    │   ├── trace_func_go.yaml
    │   ├── trace_func_py.yaml
    │   └── wimpy.yaml
    ├── firecracker
    │   ├── timed.yaml
    │   └── trace_func_go.yaml
    └── other
        ├── helloworld.yaml
        └── producer.yaml
```