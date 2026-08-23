# Nexus mixed-workload experiment modes

`run_trace_ablation.sh` uses one explicit mode at the experiment boundary:

| Mode | Trace snapshots | Khala placement |
| --- | --- | --- |
| `invm-py` | `pyaesserve-0`, `mapper-0`, `reducer-0` | guest TCP, backend `none`, Python SDK/RPC in guest |
| `nexus-go` | `gopyaesserve-s3-rpc-stream-0`, `gomapper-s3-rpc-stream-0`, `goreducer-s3-rpc-stream-0` | guest TCP selector, backend `stream`, SDK/RPC handlers in host |
| `nexus-rdma` | `gopyaesserve-s3-rpc-0`, `gomapper-s3-rpc-0`, `goreducer-s3-rpc-0` | backend `rdma`, SDK/RPC handlers in host, RDMA storage enabled |

RDMA remains a separate result. Its snapshot name intentionally has no
`-rdma` token because Khala's existing snapshot grammar selects RDMA through
backend metadata; adding a token would make the snapshot invalid.

The trace generator keeps `pyaesserve`, `mapper`, and `reducer` as canonical
identities while selecting the reference RPS and duration. It transforms names
to the matching Go snapshot only after those lookups, preventing Go names from
silently receiving the one-second fallback duration.

Inspect the complete plan without cluster or storage side effects:

```bash
./run_trace_ablation.sh --dry-run
go run experiment/khala_command.go --mode nexus-go --dry-run
```

Run the three-mode mixed/end-to-end matrix on the configured testbed:

```bash
./run_trace_ablation.sh
```

The active matrix contains no Async/prefetch or SDK-only Boolean ablations.
The four-mode synthetic latency/memory comparison, including InVM-Go and
HostTCP-Go, is driven separately by Khala's real-workload experiment harness.

## Snapshot prerequisite and exact commands

Before starting the mixed-workload run, Khala must already contain these
snapshots on every worker:

```text
invm-py:    pyaesserve-0 mapper-0 reducer-0
nexus-go:   gopyaesserve-s3-rpc-stream-0 gomapper-s3-rpc-stream-0 goreducer-s3-rpc-stream-0
nexus-rdma: gopyaesserve-s3-rpc-0 gomapper-s3-rpc-0 goreducer-s3-rpc-0
```

From the Invitro repository, create them with Khala's mode-aware command. The
default below assumes `khala` and `invitro` are sibling directories; override
`KHALA_DIR` when they are not. Each deploy/create/clean sequence requires the
real configured testbed. Cleaning with the same mode preserves the snapshots
and ensures that the `nexus-rdma` sequence also stops its RDMA server.

```bash
KHALA_DIR="${KHALA_DIR:-../khala}"
pushd "$KHALA_DIR"
for mode in invm-py nexus-go nexus-rdma; do
  go run ./experiment-cmd/khala-command --command=deploy --mode="$mode" \
    --stream-slots=4 --stream-capacity=262144
  for workload in pyaesserve mapper reducer; do
    go run ./experiment-cmd/khala-command --command=create-snapshots \
      --mode="$mode" --workload="$workload" \
      --stream-slots=4 --stream-capacity=262144
  done
  go run ./experiment-cmd/khala-command --command=clean --mode="$mode" \
    --remove-snapshots=false
done
popd
```

The trace generator requires all four scale arguments. These are the complete
per-mode equivalents of the generator calls made by the default harness:

```bash
for mode in invm-py nexus-go nexus-rdma; do
  python3 generate_trace_sweep.py --mode "$mode" \
    --divisor 100 --start-scale 1 --end-scale 27 --step 1 \
    --shift-step 10 --warmup-duration 2 --warmup-scale 1
done
```

The exact supported end-to-end entry point generates those traces, deploys and
cleans each mode, renders the loader configuration, and collects the logs:

```bash
./run_trace_ablation.sh
```

Preview the same three-mode plan without external effects with
`./run_trace_ablation.sh --dry-run`. The Nexus-RDMA clean is mode-aware and
stops the RDMA storage server. Actual execution requires Kubernetes, the Khala
workers, MinIO, the precreated snapshots, the reference trace data, and (for
the last mode) the RDMA testbed; local dry-run and unit tests do not validate
those external dependencies or performance.
