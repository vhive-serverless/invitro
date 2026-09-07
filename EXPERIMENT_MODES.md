# Nexus mixed-workload experiment modes

`run_trace_ablation.sh` uses one explicit mode at the experiment boundary. The
default B0/N4/N5 matrix is the Python-only baseline, Python frontend with the
shared-memory ring backend, and Python frontend with the RDMA/mmap-ref
backend. Go and HostTCP modes remain opt-in and are not claim-bearing here.

| Mode | Trace snapshots | Khala placement |
| --- | --- | --- |
| `invm-py` | `pyaesserve-0`, `mapper-0`, `reducer-0` | guest TCP, backend `none`, Python SDK/RPC in guest |
| `nexus-py` | `pyaesserve-s3-rpc-shmem-0`, `mapper-s3-rpc-shmem-0`, `reducer-s3-rpc-shmem-0` | Python app API, backend `shmem`, SDK/RPC handlers in host |
| `nexus-go` | `gopyaesserve-s3-rpc-shmem-0`, `gomapper-s3-rpc-shmem-0`, `goreducer-s3-rpc-shmem-0` | Go app API, backend `shmem`, SDK/RPC handlers in host |
| `nexus-rdma` | `gopyaesserve-s3-rpc-rdma-0`, `gomapper-s3-rpc-rdma-0`, `goreducer-s3-rpc-rdma-0` | backend `rdma`, SDK/RPC handlers in host, RDMA storage enabled |
| `nexus-rdma-py` | `pyaesserve-s3-rpc-rdma-0`, `mapper-s3-rpc-rdma-0`, `reducer-s3-rpc-rdma-0` | Python app API, backend `rdma` (mmap-ref), SDK/RPC/RDMA enabled |
| `hosttcp-go` | `gopyaesserve-s3-rpc-hosttcp-0`, `gomapper-s3-rpc-hosttcp-0`, `goreducer-s3-rpc-hosttcp-0` | backend `hosttcp`; SDK/signing/HTTP/TLS/gRPC/protobuf remain in guest, while DNS/TCP/opaque relay are host-owned |
| `hosttcp-py` | `pyaesserve-s3-rpc-hosttcp-0`, `mapper-s3-rpc-hosttcp-0`, `reducer-s3-rpc-hosttcp-0` | parseable functional mode with the same guest/host ownership; excluded because Python raw admission is terminally inadmissible |

RDMA remains a separate result and uses Khala's current `-s3-rpc-rdma-0`
snapshot suffix.

The trace generator keeps `pyaesserve`, `mapper`, and `reducer` as canonical
identities while selecting the reference RPS and duration. It transforms names
to the matching Go snapshot only after those lookups, preventing Go names from
silently receiving the one-second fallback duration.

Inspect the complete plan without cluster or storage side effects:

```bash
./run_trace_ablation.sh --dry-run
go run experiment/khala_command.go --mode nexus-py --dry-run
```

Run the default mixed/end-to-end matrix on the configured testbed:

```bash
./run_trace_ablation.sh
```

For a diagnostic HostTCP trace run, override the mode list explicitly:

```bash
MODE_LIST=invm-py,nexus-py,nexus-rdma-py,hosttcp-go ./run_trace_ablation.sh
```

The active matrix contains no Async/prefetch or SDK-only Boolean ablations.
The four-mode synthetic latency/memory comparison, including InVM-Go and
HostTCP-Go, is driven separately by Khala's real-workload experiment harness.

## Snapshot prerequisite and exact commands

Before starting the mixed-workload run, Khala must already contain these
snapshots on every worker:

```text
invm-py:    pyaesserve-0 mapper-0 reducer-0
nexus-py:   pyaesserve-s3-rpc-shmem-0 mapper-s3-rpc-shmem-0 reducer-s3-rpc-shmem-0
nexus-go:   gopyaesserve-s3-rpc-shmem-0 gomapper-s3-rpc-shmem-0 goreducer-s3-rpc-shmem-0
nexus-rdma: gopyaesserve-s3-rpc-rdma-0 gomapper-s3-rpc-rdma-0 goreducer-s3-rpc-rdma-0
nexus-rdma-py: pyaesserve-s3-rpc-rdma-0 mapper-s3-rpc-rdma-0 reducer-s3-rpc-rdma-0
hosttcp-go: gopyaesserve-s3-rpc-hosttcp-0 gomapper-s3-rpc-hosttcp-0 goreducer-s3-rpc-hosttcp-0
```

From the Invitro repository, create them with Khala's mode-aware command. The
default below assumes `khala` and `invitro` are sibling directories; override
`KHALA_DIR` when they are not. Each deploy/create/clean sequence requires the
real configured testbed. Cleaning with the same mode preserves the snapshots
and ensures that the `nexus-rdma` sequence also stops its RDMA server.

```bash
KHALA_DIR="${KHALA_DIR:-../khala}"
pushd "$KHALA_DIR"
for mode in invm-py nexus-py nexus-rdma-py; do
  go run ./experiment-cmd/khala-command --command=deploy --mode="$mode" \
    --shmem-ring-bytes=4190208 --shmem-io-quantum=262144 \
    --minio-endpoint=10.0.1.4:9001
  for workload in pyaesserve mapper reducer; do
    go run ./experiment-cmd/khala-command --command=create-snapshots \
      --mode="$mode" --workload="$workload" \
      --shmem-ring-bytes=4190208 --shmem-io-quantum=262144 \
      --minio-endpoint=10.0.1.4:9001
  done
  go run ./experiment-cmd/khala-command --command=clean --mode="$mode" \
    --remove-snapshots=false --minio-endpoint=10.0.1.4:9001
done
popd
```

The trace generator requires all four scale arguments. These are the complete
per-mode equivalents of the generator calls made by the default harness:

```bash
for mode in invm-py nexus-py nexus-rdma-py; do
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

Preview the same default plan without external effects with
`./run_trace_ablation.sh --dry-run`. The Nexus-RDMA clean is mode-aware and
stops the RDMA storage server. Actual execution requires Kubernetes, the Khala
workers, MinIO, the precreated snapshots, and the reference trace data. The
`nexus-rdma-py` mode also requires the RDMA testbed, while HostTCP modes require
the corresponding Khala HostTCP rootfs/configuration. Local dry-run and unit
tests do not validate those external dependencies or performance.

## Paired standalone MinIO

E3 can give every worker a dedicated S3 server on its paired storage tenant:

```bash
./run_trace_ablation.sh \
  --profile 10-node \
  --modes invm-py,nexus-py \
  --mode-order fixed \
  --reference ../b0-rps-reference.csv \
  --start-scale 1 --step 1 --end-scale 60 \
  --shift-step 10 --divisor 50 --warmup-minutes 2 \
  --repetitions 2 --cooldown-seconds 120 \
  --minio-layout paired-standalone \
  --result-root /mnt/resources/nexus-evaluation/e3-paired-RUN_ID \
  --allow-extended-end
```

`worker-node.json` defines the one-to-one pairing by array position. The runner
starts an experiment-owned `minio-standalone` tmux session on port 9000 of each
storage tenant, seeds the canonical objects independently, and passes that
tenant endpoint only to its paired worker. It leaves Kubernetes MinIO running
but does not route these cells through it. Cleanup stops only the owned tmux
session and preserves `~/minio-standalone/data`; the next deploy empties and
reseeds the benchmark buckets.

Each tenant must have the pinned executable at `~/minio-binaries/minio`. The
runner verifies SHA-256
`aa479bd2456d0722a6737f82a1cf193aa60f934269573a6a7e024aebe1069243`
before acquisition and records the hash and per-tenant endpoint in remote
provenance. A missing or different binary, incomplete pairing, failed health
check, or failed object seed is a setup failure and prevents that cell from
starting. This layout changes both the distributed-MinIO topology and the
Kubernetes/Istio access path, so a performance difference cannot by itself be
attributed solely to distributed MinIO.
