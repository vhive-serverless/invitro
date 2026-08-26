#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
cd "$repo_root"

profile=
modes_csv=invm-py,nexus-py,nexus-rdma-py
reference=
start_scale=1
step=1
end_scale=27
warmup_minutes=2
repetitions=3
shift_step=10
divisor=100
cooldown_seconds=120
result_root=
minio_endpoint=
claim_run=false
allow_extended_end=false
dry_run=false
active_sampler_pid=
active_stop_file=

stop_active_sampler() {
    if [[ -n "$active_sampler_pid" ]]; then
        : > "$active_stop_file"
        wait "$active_sampler_pid" 2>/dev/null || true
        active_sampler_pid=
        active_stop_file=
    fi
}
trap stop_active_sampler EXIT INT TERM

usage() {
    cat <<'EOF'
Usage: run_trace_ablation.sh --profile 4-node|18-node
  --modes invm-py,nexus-py,nexus-rdma-py --reference b0-rps-reference.csv
  --start-scale 1 --step 1 --end-scale 27 --warmup-minutes 2
  --repetitions 3 --result-root PATH [--claim-run] [--allow-extended-end] [--dry-run]

The 18-node paper acquisition requires --claim-run. A 4-node run is a
non-claiming integration preflight, not a node-count scale point.
EOF
}

while (($#)); do
    case "$1" in
        --profile) profile=${2:?}; shift 2 ;;
        --modes) modes_csv=${2:?}; shift 2 ;;
        --reference) reference=${2:?}; shift 2 ;;
        --start-scale) start_scale=${2:?}; shift 2 ;;
        --step) step=${2:?}; shift 2 ;;
        --end-scale) end_scale=${2:?}; shift 2 ;;
        --warmup-minutes) warmup_minutes=${2:?}; shift 2 ;;
        --repetitions) repetitions=${2:?}; shift 2 ;;
        --shift-step) shift_step=${2:?}; shift 2 ;;
        --divisor) divisor=${2:?}; shift 2 ;;
        --cooldown-seconds) cooldown_seconds=${2:?}; shift 2 ;;
        --result-root) result_root=${2:?}; shift 2 ;;
        --minio-endpoint) minio_endpoint=${2:?}; shift 2 ;;
        --claim-run) claim_run=true; shift ;;
        --allow-extended-end) allow_extended_end=true; shift ;;
        --dry-run) dry_run=true; shift ;;
        -h|--help) usage; exit 0 ;;
        *) echo "unknown option: $1" >&2; usage >&2; exit 2 ;;
    esac
done

[[ "$profile" == 4-node || "$profile" == 18-node ]] || { echo "--profile must be 4-node or 18-node" >&2; exit 2; }
[[ -f "$reference" ]] || { echo "--reference must name the frozen B0 RPS reference" >&2; exit 2; }
[[ -n "$result_root" ]] || { echo "--result-root is required" >&2; exit 2; }
for value in "$start_scale" "$step" "$end_scale" "$warmup_minutes" "$repetitions" "$shift_step" "$divisor" "$cooldown_seconds"; do
    [[ "$value" =~ ^[0-9]+$ ]] || { echo "scale, timing, and repetition values must be nonnegative integers" >&2; exit 2; }
done
((start_scale > 0 && step > 0 && end_scale >= start_scale && repetitions > 0 && divisor > 0)) || {
    echo "invalid scale/repetition contract" >&2; exit 2; }
[[ "$warmup_minutes" == 2 ]] || { echo "E3/E4 requires a two-minute warmup" >&2; exit 2; }
if ((end_scale > 27)) && [[ "$allow_extended_end" != true ]]; then
    echo "END_SCALE above 27 requires explicit --allow-extended-end" >&2
    exit 2
fi

IFS=',' read -r -a modes <<< "$modes_csv"
expected_modes=(invm-py nexus-py nexus-rdma-py)
[[ ${#modes[@]} -eq 3 ]] || { echo "E3/E4 requires exactly B0/N4/N5" >&2; exit 2; }
for expected in "${expected_modes[@]}"; do
    [[ ",$modes_csv," == *",$expected,"* ]] || { echo "missing E3/E4 mode $expected" >&2; exit 2; }
done

if [[ "$profile" == 18-node ]]; then
    [[ "$claim_run" == true ]] || { echo "18-node acquisition requires explicit --claim-run" >&2; exit 2; }
    [[ "$start_scale" == 1 && "$step" == 1 && "$repetitions" == 3 ]] || {
        echo "18-node claim run requires START=1 STEP=1 and three repetitions" >&2; exit 2; }
    if [[ "$allow_extended_end" != true && "$end_scale" != 27 ]]; then
        echo "initial 18-node claim run requires END=27" >&2; exit 2
    fi
    minio_endpoint=${minio_endpoint:-http://myminio-api.minio.10.200.3.4.sslip.io}
else
    [[ "$claim_run" == false ]] || { echo "4-node preflight cannot be marked claim-bearing" >&2; exit 2; }
    minio_endpoint=${minio_endpoint:-10.0.1.4:9001}
fi

python3 - "$reference" <<'PY'
import sys
from pathlib import Path
from generate_trace_sweep import read_e2_reference
read_e2_reference(Path(sys.argv[1]))
PY

rotate() {
    local -n values=$1
    local offset=$(( $2 % ${#values[@]} )) index
    for ((index=offset; index<${#values[@]}; index++)); do printf '%s ' "${values[index]}"; done
    for ((index=0; index<offset; index++)); do printf '%s ' "${values[index]}"; done
}

require_clean_repo() {
    local path=$1 label=$2 status
    [[ -d "$path/.git" ]] || { echo "missing $label repository at $path" >&2; exit 2; }
    status=$(git -C "$path" status --short)
    [[ -z "$status" ]] || { echo "$label repository is dirty; refusing acquisition" >&2; printf '%s\n' "$status" >&2; exit 2; }
}

discover_topology() {
    local inventory_path=$1 worker_config_path=$2 expected_workers expected_tenants
    if [[ "$profile" == 4-node ]]; then expected_workers=1; expected_tenants=1; else expected_workers=8; expected_tenants=8; fi
    kubectl get nodes -o wide --show-labels > "$inventory_path"
    local master_count loader_count worker_count tenant_count node_count
    master_count=$(kubectl get nodes -l loader-nodetype=master --no-headers 2>/dev/null | wc -l)
    loader_count=$(kubectl get nodes -l loader-nodetype=monitoring --no-headers 2>/dev/null | wc -l)
    worker_count=$(kubectl get nodes -l loader-nodetype=worker --no-headers 2>/dev/null | wc -l)
    tenant_count=$(kubectl get nodes -l minio-type=tenant --no-headers 2>/dev/null | wc -l)
    node_count=$(kubectl get nodes --no-headers | wc -l)
    [[ "$master_count" == 1 && "$loader_count" == 1 && "$worker_count" == "$expected_workers" && "$tenant_count" == "$expected_tenants" ]] || {
        echo "live labels do not match $profile: master=$master_count loader=$loader_count worker=$worker_count tenant=$tenant_count" >&2
        return 2
    }
    [[ "$node_count" == "${profile%-node}" ]] || { echo "live node count $node_count does not match $profile" >&2; return 2; }
    mapfile -t workers < <(kubectl get nodes -l loader-nodetype=worker -o jsonpath='{range .items[*]}{.status.addresses[?(@.type=="InternalIP")].address}{"\n"}{end}' | sort -V)
    mapfile -t tenants < <(kubectl get nodes -l minio-type=tenant -o jsonpath='{range .items[*]}{.status.addresses[?(@.type=="InternalIP")].address}{"\n"}{end}' | sort -V)
    workers_csv=$(IFS=,; echo "${workers[*]}")
    tenants_csv=$(IFS=,; echo "${tenants[*]}")
    jq -n --arg workers "$workers_csv" --arg storage "$tenants_csv" \
        '{worker_nodes:($workers|split(",")),storage_nodes:($storage|split(","))}' > "$worker_config_path"
}

write_config() {
    local experiment=$1 trace_path=$2 output_prefix=$3 destination=$4
    EXPERIMENT="$experiment" EXP_DUR="$end_scale" WARMUP="$warmup_minutes" PREFETCH=false \
        ENABLE_PERF=false FIXED_REPLICAS=0 TRACE_PATH="$trace_path" OUTPUT_PREFIX="$output_prefix" \
        envsubst < cmd/config_khala_trace_template.json > "$destination"
}

run_cell() {
    local repetition=$1 mode=$2 worker_config=$3 workers_for_sampler=$4
    local run_id="e3-e4-r${repetition}-${mode}"
    local scratch_trace="data/traces/nexus-e3-e4/$run_id"
    local scratch_out="data/out/nexus-e3-e4/$run_id"
    local destination="$result_root/rep-$repetition/$mode"
    local manifest="$destination/manifest.txt"
    if [[ -f "$manifest" ]] && grep -Fqx 'exit_status=0' "$manifest"; then
        echo "RESUME skip $run_id"
        return
    fi
    [[ ! -e "$destination" ]] || { echo "refusing incomplete cell: $destination" >&2; return 2; }
    rm -rf -- "$scratch_trace" "$scratch_out"
    mkdir -p "$scratch_out"
    python3 generate_trace_sweep.py --mode "$mode" --e2-reference "$reference" \
        --divisor "$divisor" --start-scale "$start_scale" --end-scale "$end_scale" \
        --step "$step" --shift-step "$shift_step" --warmup-duration "$warmup_minutes" \
        --warmup-scale 1 --output "$scratch_trace" > "$scratch_out/trace-generator.log" 2>&1
    cp -a -- "$scratch_trace" "$scratch_out/trace"
    cp -- "$worker_config" "$scratch_out/worker-node.json"
    local config_path="$scratch_out/config.json"
    write_config "$run_id" "$scratch_trace" "$scratch_out/experiment" "$config_path"
    {
        echo manifest_version=1
        echo experiment=e3-e4
        echo "claim_bearing=$claim_run"
        echo "profile=$profile"
        echo "repetition=$repetition"
        echo "mode=$mode"
        echo "python_workloads=chameleonserve cnnserve imageresize lrserving mapper pyaesserve reducer rnnserve streducer sttrainer"
        echo "deployed_function_rows=$((10 * end_scale))"
        echo "start_scale=$start_scale"
        echo "step=$step"
        echo "end_scale=$end_scale"
        echo "explicit_extended_end=$allow_extended_end"
        echo "warmup_minutes=$warmup_minutes"
        echo "measurement_minutes=$end_scale"
        echo "perf_enabled=false"
        echo "min_scale=0"
        echo "minio_endpoint=$minio_endpoint"
        echo "workers=$workers_for_sampler"
        echo "start_utc=$(date -u --iso-8601=seconds)"
        for pair in "invitro:." "khala:../khala" "firecracker:../firecracker" "rdma_demo:../rdma-demo"; do
            label=${pair%%:*}; path=${pair#*:}
            echo "${label}_head=$(git -C "$path" rev-parse HEAD)"
            echo "${label}_branch=$(git -C "$path" branch --show-current)"
            echo "${label}_status=$(git -C "$path" status --short | tr '\n' '|')"
        done
        sha256sum "$reference" generate_trace_sweep.py collect_e4_memory.py run_trace_ablation.sh \
            data/traces/reference/preprocessed_150.tar.gz \
            "$scratch_trace/invocations.csv" "$scratch_trace/durations.csv" "$config_path" "$worker_config"
    } > "$scratch_out/manifest.txt"

    local status=0 clean_status=0 sampler_status=0 sampler_pid= stop_file="$scratch_out/stop-memory"
    set +e
    go run experiment/khala_command.go --command deploy --mode "$mode" --worker-config "$worker_config" \
        --shmem-ring-bytes 4190208 --shmem-io-quantum 262144 --minio-endpoint "$minio_endpoint" \
        > >(tee "$scratch_out/deploy.log") 2>&1
    status=$?
    if ((status == 0)); then
        python3 collect_e4_memory.py --workers "$workers_for_sampler" --mode "$mode" --repetition "$repetition" \
            --stop-file "$stop_file" --output "$scratch_out/firecracker-memory.csv" \
            --backend-output "$scratch_out/backend-memory-once.csv" > "$scratch_out/memory-sampler.log" 2>&1 &
        sampler_pid=$!
        active_sampler_pid=$sampler_pid
        active_stop_file=$stop_file
        go run cmd/loader.go --config "$config_path" > >(tee "$scratch_out/loader.log") 2>&1
        status=$?
        : > "$stop_file"
        wait "$sampler_pid"
        sampler_status=$?
        active_sampler_pid=
        active_stop_file=
        if ((status == 0 && sampler_status != 0)); then status=$sampler_status; fi
        if rg --quiet ',error:' "$scratch_out/firecracker-memory.csv" "$scratch_out/backend-memory-once.csv"; then
            echo "memory sampler recorded an SSH/parse failure" >> "$scratch_out/memory-sampler.log"
            if ((status == 0)); then status=2; fi
        fi
    fi
    kubectl logs deployment/activator -n knative-serving > "$scratch_out/activator.log" 2>&1
    go run experiment/khala_command.go --command clean --mode "$mode" --worker-config "$worker_config" \
        --minio-endpoint "$minio_endpoint" --remove-snapshots=false > "$scratch_out/clean.log" 2>&1
    clean_status=$?
    if ((status == 0 && clean_status != 0)); then status=$clean_status; fi
    set -e
    {
        echo "end_utc=$(date -u --iso-8601=seconds)"
        echo "sampler_exit_status=$sampler_status"
        echo "cleanup_exit_status=$clean_status"
        echo "exit_status=$status"
    } >> "$scratch_out/manifest.txt"
    mkdir -p "$(dirname "$destination")"
    cp -a -- "$scratch_out" "$destination"
    rm -rf -- "$scratch_trace" "$scratch_out"
    if ((status != 0)); then echo "cell failed; evidence retained at $destination" >&2; return "$status"; fi
    if ((cooldown_seconds > 0)); then sleep "$cooldown_seconds"; fi
}

function_count=$((10 * end_scale))
total_minutes=$((warmup_minutes + end_scale))
for ((repetition=0; repetition<repetitions; repetition++)); do
    read -r -a rotated_modes <<< "$(rotate modes "$repetition")"
    for mode in "${rotated_modes[@]}"; do
        printf 'CELL experiment=e3-e4 profile=%s claim_bearing=%s repetition=%d mode=%s workloads=10 deployed_function_rows=%d warmup_minutes=%d measurement_minutes=%d perf=false output=%s\n' \
            "$profile" "$claim_run" "$repetition" "$mode" "$function_count" "$warmup_minutes" "$end_scale" "$result_root/rep-$repetition/$mode"
    done
done
echo "PLAN profile=$profile modes=${#modes[@]} repetitions=$repetitions deployed_function_rows=$function_count total_minutes_per_cell=$total_minutes auto_extend=false minio_endpoint=$minio_endpoint"
if [[ "$dry_run" == true ]]; then
    for mode in "${modes[@]}"; do
        python3 generate_trace_sweep.py --mode "$mode" --e2-reference "$reference" \
            --divisor "$divisor" --start-scale "$start_scale" --end-scale "$end_scale" \
            --step "$step" --shift-step "$shift_step" --warmup-duration "$warmup_minutes" \
            --warmup-scale 1 --dry-run > /dev/null
    done
    echo "E3_E4_DRY_RUN_READY"
    exit 0
fi

require_clean_repo . invitro
require_clean_repo ../khala khala
require_clean_repo ../firecracker firecracker
require_clean_repo ../rdma-demo rdma-demo
if [[ -e "$result_root" ]]; then
    [[ -f "$result_root/worker-node.json" && -f "$result_root/cluster-inventory.txt" && -f "$result_root/b0-rps-reference.csv" ]] || {
        echo "existing result root lacks resume provenance" >&2; exit 2; }
    cmp --silent "$reference" "$result_root/b0-rps-reference.csv" || {
        echo "E2 reference differs from the interrupted run" >&2; exit 2; }
    resume_check=$(mktemp -d)
    discover_topology "$resume_check/cluster-inventory.txt" "$resume_check/worker-node.json"
    cmp --silent "$resume_check/worker-node.json" "$result_root/worker-node.json" || {
        echo "live worker/storage pairing differs from the interrupted run" >&2
        rm -r -- "$resume_check"
        exit 2
    }
    rm -r -- "$resume_check"
    workers_csv=$(jq -r '.worker_nodes | join(",")' "$result_root/worker-node.json")
else
    mkdir -p "$result_root"
    discover_topology "$result_root/cluster-inventory.txt" "$result_root/worker-node.json"
    cp -- "$reference" "$result_root/b0-rps-reference.csv"
fi

for ((repetition=0; repetition<repetitions; repetition++)); do
    read -r -a rotated_modes <<< "$(rotate modes "$repetition")"
    for mode in "${rotated_modes[@]}"; do
        run_cell "$repetition" "$mode" "$result_root/worker-node.json" "$workers_csv"
    done
done
echo "E3_E4_ACQUISITION_READY result_root=$result_root"
