#!/usr/bin/env bash
set -euo pipefail

# Mixed/end-to-end attribution matrix: B0 (Python InVM), N4 (Python plus
# shmem ring), and N5 (Python plus RDMA/mmap-ref). Go and HostTCP modes remain
# available as explicit diagnostics.
MODE_LIST="${MODE_LIST:-invm-py,nexus-py,nexus-rdma-py}"
IFS=',' read -r -a MODES <<< "$MODE_LIST"
MAX_MULTIPLIER="${MAX_MULTIPLIER:-27}"
DIVISOR="${DIVISOR:-100}"
EXP_WARMUP="${EXP_WARMUP:-2}"
START_SCALE="${START_SCALE:-1}"
STEP="${STEP:-1}"
SHIFT_STEP="${SHIFT_STEP:-10}"
COOLDOWN_SECONDS="${COOLDOWN_SECONDS:-120}"
SHMEM_RING_BYTES="${SHMEM_RING_BYTES:-4190208}"
SHMEM_IO_QUANTUM="${SHMEM_IO_QUANTUM:-262144}"
MINIO_ENDPOINT="${MINIO_ENDPOINT:-10.0.1.4:9001}"
DRY_RUN=false

usage() {
    cat <<'USAGE'
Usage: run_trace_ablation.sh [--dry-run]

Environment overrides: MAX_MULTIPLIER, DIVISOR, EXP_WARMUP, START_SCALE,
STEP, SHIFT_STEP, COOLDOWN_SECONDS, MODE_LIST, SHMEM_RING_BYTES,
SHMEM_IO_QUANTUM, MINIO_ENDPOINT.

--dry-run prints the complete default plan without writing traces or
contacting Kubernetes, MinIO, worker nodes, or RDMA storage.
USAGE
}

while (($#)); do
    case "$1" in
        --dry-run) DRY_RUN=true ;;
        -h|--help) usage; exit 0 ;;
        *) echo "unknown argument: $1" >&2; usage >&2; exit 2 ;;
    esac
    shift
done

experiment_duration=$(( (MAX_MULTIPLIER - START_SCALE) / STEP + 1 ))

trace_command() {
    local mode="$1"
    printf '%q ' python3 generate_trace_sweep.py \
        --mode "$mode" \
        --divisor "$DIVISOR" \
        --start-scale "$START_SCALE" \
        --end-scale "$MAX_MULTIPLIER" \
        --step "$STEP" \
        --shift-step "$SHIFT_STEP" \
        --warmup-duration "$EXP_WARMUP" \
        --warmup-scale 1
}

khala_command() {
    local command="$1"
    local mode="$2"
    if [[ "$command" == clean ]]; then
        printf '%q ' go run experiment/khala_command.go \
            --command "$command" \
            --mode "$mode" \
            --minio-endpoint "$MINIO_ENDPOINT"
    else
        printf '%q ' go run experiment/khala_command.go \
            --command "$command" \
            --mode "$mode" \
            --shmem-ring-bytes "$SHMEM_RING_BYTES" \
            --shmem-io-quantum "$SHMEM_IO_QUANTUM" \
            --minio-endpoint "$MINIO_ENDPOINT"
    fi
}

mode_metadata() {
    case "$1" in
        invm-py)
            echo "backend=none sdk=guest rpc=guest rdma=false snapshots=pyaesserve-0,mapper-0,reducer-0"
            ;;
        nexus-py)
            echo "backend=shmem sdk=host rpc=host rdma=false snapshots=pyaesserve-s3-rpc-shmem-0,mapper-s3-rpc-shmem-0,reducer-s3-rpc-shmem-0"
            ;;
        nexus-go)
            echo "backend=shmem sdk=host rpc=host rdma=false snapshots=gopyaesserve-s3-rpc-shmem-0,gomapper-s3-rpc-shmem-0,goreducer-s3-rpc-shmem-0"
            ;;
        nexus-rdma)
            echo "backend=rdma sdk=host rpc=host rdma=true snapshots=gopyaesserve-s3-rpc-rdma-0,gomapper-s3-rpc-rdma-0,goreducer-s3-rpc-rdma-0"
            ;;
        nexus-rdma-py)
            echo "backend=rdma sdk=host rpc=host rdma=true snapshots=pyaesserve-s3-rpc-rdma-0,mapper-s3-rpc-rdma-0,reducer-s3-rpc-rdma-0"
            ;;
        hosttcp-go)
            echo "backend=hosttcp guest-owned=sdk,signing,http,tls,grpc,protobuf host-owned=dns,tcp,opaque-relay rdma=false snapshots=gopyaesserve-s3-rpc-hosttcp-0,gomapper-s3-rpc-hosttcp-0,goreducer-s3-rpc-hosttcp-0"
            ;;
        hosttcp-py)
            echo "backend=hosttcp guest-owned=sdk,signing,http,tls,grpc,protobuf host-owned=dns,tcp,opaque-relay rdma=false snapshots=pyaesserve-s3-rpc-hosttcp-0,mapper-s3-rpc-hosttcp-0,reducer-s3-rpc-hosttcp-0"
            ;;
        *) return 2 ;;
    esac
}

validate_mode() {
    case "$1" in
        invm-py|nexus-py|nexus-go|nexus-rdma|nexus-rdma-py|hosttcp-go|hosttcp-py) ;;
        *) echo "unsupported mode: $1" >&2; return 2 ;;
    esac
}

run_mode() {
    local mode="$1"
    local experiment="${mode}_d-${DIVISOR}_s-${START_SCALE}_e-${MAX_MULTIPLIER}_t-${STEP}"

    validate_mode "$mode"

    echo "=== mode=${mode} experiment=${experiment} ==="
    if [[ "$DRY_RUN" == true ]]; then
        echo "PLACEMENT: $(mode_metadata "$mode")"
        echo "TRACE: $(trace_command "$mode")"
        echo "DEPLOY: $(khala_command deploy "$mode")"
        echo "CONFIG: EXPERIMENT=${experiment} EXP_DUR=${experiment_duration} WARMUP=${EXP_WARMUP} PREFETCH=false envsubst < cmd/config_khala_trace_template.json > cmd/config_khala_trace.json"
        echo "LOAD: go run cmd/loader.go --config cmd/config_khala_trace.json"
        echo "LOGS: kubectl logs deployment/activator -n knative-serving"
        echo "CLEAN: $(khala_command clean "$mode")--remove-snapshots=false"
        return
    fi

    python3 generate_trace_sweep.py \
        --mode "$mode" \
        --divisor "$DIVISOR" \
        --start-scale "$START_SCALE" \
        --end-scale "$MAX_MULTIPLIER" \
        --step "$STEP" \
        --shift-step "$SHIFT_STEP" \
        --warmup-duration "$EXP_WARMUP" \
        --warmup-scale 1

    mkdir -p "data/out/${experiment}"
    go run experiment/khala_command.go \
        --command deploy \
        --mode "$mode" \
        --shmem-ring-bytes "$SHMEM_RING_BYTES" \
        --shmem-io-quantum "$SHMEM_IO_QUANTUM" \
        --minio-endpoint "$MINIO_ENDPOINT"
    EXPERIMENT="$experiment" EXP_DUR="$experiment_duration" WARMUP="$EXP_WARMUP" PREFETCH=false \
        envsubst < cmd/config_khala_trace_template.json > cmd/config_khala_trace.json
    go run cmd/loader.go --config cmd/config_khala_trace.json | tee "data/out/${experiment}/loader.log"
    kubectl logs deployment/activator -n knative-serving > "data/out/${experiment}/activator.log"
    go run experiment/khala_command.go \
        --command clean \
        --mode "$mode" \
        --minio-endpoint "$MINIO_ENDPOINT" \
        --remove-snapshots=false
    sleep "$COOLDOWN_SECONDS"
}

for mode in "${MODES[@]}"; do
    validate_mode "$mode"
done

for mode in "${MODES[@]}"; do
    run_mode "$mode"
done
