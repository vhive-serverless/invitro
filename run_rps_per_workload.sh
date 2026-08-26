#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
cd "$repo_root"

command=${1:-}
if [[ -n "$command" ]]; then shift; fi
profile=4-node
e1_summary=
reference=
worker_cores=
slo_multiplier=5
failure_threshold=0.05
warmup_minutes=2
steps=20
minutes_per_step=1
measurement_minutes=3
replicas=320
repetitions=3
result_root=
minio_endpoint=10.0.1.4:9001
dry_run=false
no_retry=false

usage() {
    cat <<'EOF'
Usage:
  run_rps_per_workload.sh calibrate --profile 4-node --e1-summary FILE --worker-cores N
      --slo-multiplier 5 --failure-threshold 0.05 --warmup-minutes 2 --steps 20
      --minutes-per-step 1 --no-retry --result-root PATH [--dry-run]
  run_rps_per_workload.sh collect --profile 4-node --reference FILE --replicas 320
      --repetitions 3 --result-root PATH [--dry-run]
EOF
}

while (($#)); do
    case "$1" in
        --profile) profile=${2:?}; shift 2 ;;
        --e1-summary) e1_summary=${2:?}; shift 2 ;;
        --reference) reference=${2:?}; shift 2 ;;
        --worker-cores) worker_cores=${2:?}; shift 2 ;;
        --slo-multiplier) slo_multiplier=${2:?}; shift 2 ;;
        --failure-threshold) failure_threshold=${2:?}; shift 2 ;;
        --warmup-minutes) warmup_minutes=${2:?}; shift 2 ;;
        --steps) steps=${2:?}; shift 2 ;;
        --minutes-per-step) minutes_per_step=${2:?}; shift 2 ;;
        --measurement-minutes) measurement_minutes=${2:?}; shift 2 ;;
        --replicas) replicas=${2:?}; shift 2 ;;
        --repetitions) repetitions=${2:?}; shift 2 ;;
        --result-root) result_root=${2:?}; shift 2 ;;
        --minio-endpoint) minio_endpoint=${2:?}; shift 2 ;;
        --no-retry) no_retry=true; shift ;;
        --dry-run) dry_run=true; shift ;;
        -h|--help) usage; exit 0 ;;
        *) echo "unknown option: $1" >&2; usage >&2; exit 2 ;;
    esac
done

[[ "$command" == calibrate || "$command" == collect ]] || { usage >&2; exit 2; }
[[ "$profile" == 4-node ]] || { echo "E2 supports only the frozen 4-node profile" >&2; exit 2; }
[[ -n "$result_root" ]] || { echo "--result-root is required" >&2; exit 2; }
[[ "$warmup_minutes" == 2 ]] || { echo "E2 requires --warmup-minutes 2" >&2; exit 2; }
[[ "$minio_endpoint" == 10.0.1.4:9001 ]] || { echo "E2 requires MinIO 10.0.1.4:9001" >&2; exit 2; }

workloads=(helloworld chameleonserve cnnserve imageresize lrserving mapper pyaesserve reducer rnnserve streducer sttrainer)
python_modes=(invm-py nexus-py nexus-rdma-py)
hello_extra_modes=(invm-go invm-js hosttcp-go nexus-go nexus-js)

rotate() {
    local -n values=$1
    local offset=$(( $2 % ${#values[@]} )) index
    for ((index=offset; index<${#values[@]}; index++)); do printf '%s ' "${values[index]}"; done
    for ((index=0; index<offset; index++)); do printf '%s ' "${values[index]}"; done
}

reference_value() {
    local workload=$1 column=$2
    python3 - "$reference" "$workload" "$column" <<'PY'
import csv, sys
path, workload, column = sys.argv[1:]
with open(path, newline='', encoding='utf-8') as handle:
    rows = list(csv.DictReader(handle))
matches = [row for row in rows if row.get('workload') == workload]
if len(matches) != 1 or column not in matches[0]:
    raise SystemExit(f'missing unique {workload}/{column} in {path}')
row = matches[0]
if row.get('status') != 'BOUNDARY_OBSERVED' or not row[column]:
    raise SystemExit(f'{workload} is not admissible for collection: status={row.get("status")}')
print(row[column])
PY
}

reference_unique_value() {
    local column=$1
    python3 - "$reference" "$column" <<'PY'
import csv, sys
path, column = sys.argv[1:]
with open(path, newline='', encoding='utf-8') as handle:
    rows = list(csv.DictReader(handle))
values = {row.get(column, '') for row in rows}
if len(values) != 1 or '' in values:
    raise SystemExit(f'{path} does not contain one unique {column}: {sorted(values)}')
print(values.pop())
PY
}

require_clean_repo() {
    local path=$1 label=$2
    [[ -d "$path/.git" ]] || { echo "missing $label repository at $path" >&2; exit 2; }
    local status
    status=$(git -C "$path" status --short)
    [[ -z "$status" ]] || { echo "$label repository is dirty; refusing claim-bearing run" >&2; printf '%s\n' "$status" >&2; exit 2; }
}

validate_claim_sources() {
    require_clean_repo . invitro
    require_clean_repo ../khala khala
    require_clean_repo ../firecracker firecracker
    require_clean_repo ../rdma-demo rdma-demo
}

print_cell() {
    printf 'CELL phase=%s repetition=%s mode=%s workload=%s rps=%s replicas=%s perf=%s warmup_minutes=%s measurement_minutes=%s output=%s\n' "$@"
}

write_config() {
    local experiment=$1 duration=$2 perf=$3 fixed=$4 trace_path=$5 output_prefix=$6 destination=$7
    EXPERIMENT="$experiment" EXP_DUR="$duration" WARMUP="$warmup_minutes" PREFETCH=false \
        ENABLE_PERF="$perf" FIXED_REPLICAS="$fixed" TRACE_PATH="$trace_path" \
        OUTPUT_PREFIX="$output_prefix" \
        envsubst < cmd/config_khala_trace_template.json > "$destination"
}

run_cell() {
    local phase=$1 repetition=$2 mode=$3 workload=$4 rps=$5 perf=$6 duration=$7 destination=$8
    local run_id="e2-${phase}-r${repetition}-${mode}-${workload}"
    local scratch_trace="data/traces/nexus-e2/$run_id"
    local scratch_out="data/out/nexus-e2/$run_id"
    local config_path="$scratch_out/config.json"
    local manifest="$destination/manifest.txt"
    if [[ -f "$manifest" ]] && grep -Fqx 'exit_status=0' "$manifest"; then
        echo "RESUME skip $run_id"
        return
    fi
    [[ ! -e "$destination" ]] || { echo "refusing incomplete cell: $destination" >&2; return 2; }
    rm -rf -- "$scratch_out" "$scratch_trace"
    mkdir -p "$scratch_out"
    if [[ "$phase" == calibration ]]; then
        python3 e2_calibrate_rps.py --averages "$e1_summary" --cores "$worker_cores" trace \
            --workload "$workload" --warmup-minutes "$warmup_minutes" --output "$scratch_trace"
    else
        python3 e2_calibrate_rps.py --averages "$e1_summary" --cores "$worker_cores" fixed-trace \
            --workload "$workload" --mode "$mode" --rps "$rps" --warmup-minutes "$warmup_minutes" \
            --measurement-minutes "$duration" --output "$scratch_trace"
    fi
    write_config "$run_id" "$duration" "$perf" "$replicas" "$scratch_trace" "$scratch_out/experiment" "$config_path"
    cp -a -- "$scratch_trace" "$scratch_out/trace"
    {
        echo manifest_version=1
        echo "phase=$phase"
        echo "repetition=$repetition"
        echo "mode=$mode"
        echo "workload=$workload"
        echo "rps=$rps"
        echo "replicas=$replicas"
        echo "perf_enabled=$perf"
        echo "start_utc=$(date -u --iso-8601=seconds)"
        echo "invitro_head=$(git rev-parse HEAD)"
        echo "invitro_branch=$(git branch --show-current)"
        echo "invitro_status=$(git status --short | tr '\n' '|')"
        echo "khala_head=$(git -C ../khala rev-parse HEAD)"
        echo "khala_branch=$(git -C ../khala branch --show-current)"
        echo "khala_status=$(git -C ../khala status --short | tr '\n' '|')"
        echo "firecracker_head=$(git -C ../firecracker rev-parse HEAD)"
        echo "firecracker_branch=$(git -C ../firecracker branch --show-current)"
        echo "firecracker_status=$(git -C ../firecracker status --short | tr '\n' '|')"
        echo "rdma_demo_head=$(git -C ../rdma-demo rev-parse HEAD)"
        echo "rdma_demo_branch=$(git -C ../rdma-demo branch --show-current)"
        echo "rdma_demo_status=$(git -C ../rdma-demo status --short | tr '\n' '|')"
        echo "profile=$profile"
        echo "minio_endpoint=$minio_endpoint"
        echo "warmup_minutes=$warmup_minutes"
        echo "measurement_minutes=$duration"
        echo "slo_multiplier=$slo_multiplier"
        echo "failure_threshold=$failure_threshold"
        echo "worker_cores=$worker_cores"
        sha256sum "$e1_summary" e2_calibrate_rps.py run_rps_per_workload.sh trace_modes.py \
            "$scratch_trace/invocations.csv" "$scratch_trace/durations.csv" "$config_path"
        if [[ -f "$reference" ]]; then sha256sum "$reference"; fi
    } > "$scratch_out/manifest.txt"
    local status=0
    set +e
    go run experiment/khala_command.go --command deploy --mode "$mode" --workloads "$workload" \
        --shmem-ring-bytes 4190208 --shmem-io-quantum 262144 --minio-endpoint "$minio_endpoint" \
        > >(tee "$scratch_out/deploy.log") 2>&1
    status=$?
    if ((status == 0)); then
        go run cmd/loader.go --config "$config_path" > >(tee "$scratch_out/loader.log") 2>&1
        status=$?
    fi
    go run experiment/khala_command.go --command clean --mode "$mode" --minio-endpoint "$minio_endpoint" \
        --remove-snapshots=false > "$scratch_out/clean.log" 2>&1
    clean_status=$?
    if ((status == 0 && clean_status != 0)); then status=$clean_status; fi
    set -e
    {
        echo "end_utc=$(date -u --iso-8601=seconds)"
        echo "exit_status=$status"
    } >> "$scratch_out/manifest.txt"
    mkdir -p "$(dirname "$destination")"
    cp -a -- "$scratch_out" "$destination"
    rm -rf -- "$scratch_out" "$scratch_trace"
    if ((status != 0)); then
        echo "cell failed; evidence retained at $destination" >&2
        return "$status"
    fi
}

if [[ "$command" == calibrate ]]; then
    [[ -f "$e1_summary" && -n "$worker_cores" ]] || { echo "calibrate requires --e1-summary and --worker-cores" >&2; exit 2; }
    [[ "$slo_multiplier" == 5 && "$failure_threshold" == 0.05 && "$steps" == 20 && "$minutes_per_step" == 1 && "$no_retry" == true ]] || {
        echo "calibration contract is frozen at 5x, >5%, 20 one-minute steps, and no retry" >&2; exit 2; }
    plan_path="$result_root/calibration-plan.csv"
    if [[ "$dry_run" == true ]]; then
        python3 - "$e1_summary" "$worker_cores" <<'PY' >/dev/null
import sys
from pathlib import Path
from e2_calibrate_rps import build_plan, read_averages
build_plan(read_averages(Path(sys.argv[1])), int(sys.argv[2]))
PY
        for workload in "${workloads[@]}"; do
            print_cell calibration 0 invm-py "$workload" sweep 320 false 2 20 "$result_root/cells/$workload"
        done
        exit 0
    fi
    validate_claim_sources
    mkdir -p "$result_root"
    python3 e2_calibrate_rps.py --averages "$e1_summary" --cores "$worker_cores" plan --output "$plan_path"
    observations=()
    for workload in "${workloads[@]}"; do
        cell="$result_root/cells/$workload"
        run_cell calibration 0 invm-py "$workload" sweep false 20 "$cell"
        duration_csv=$(find "$cell" -maxdepth 1 -name 'experiment_duration_*.csv' -print -quit)
        [[ -n "$duration_csv" ]] || { echo "missing duration CSV for $workload" >&2; exit 2; }
        observation="$cell/observations.csv"
        python3 e2_calibrate_rps.py --averages "$e1_summary" --cores "$worker_cores" observe \
            --workload "$workload" --duration-csv "$duration_csv" --output "$observation"
        observations+=("$observation")
    done
    python3 e2_calibrate_rps.py --averages "$e1_summary" --cores "$worker_cores" finalize \
        --observations "${observations[@]}" --output "$result_root/b0-rps-reference.csv"
    echo "CALIBRATION_READY reference=$result_root/b0-rps-reference.csv"
    exit 0
fi

[[ -f "$reference" ]] || { echo "collect requires --reference" >&2; exit 2; }
[[ "$replicas" == 320 && "$repetitions" == 3 ]] || { echo "collection requires 320 replicas and three repetitions" >&2; exit 2; }
if [[ -z "$e1_summary" ]]; then
    e1_summary=${E1_SUMMARY:-$(dirname "$reference")/../e1-real/analysis/b0-unloaded-average.csv}
fi
[[ -f "$e1_summary" ]] || { echo "set E1_SUMMARY to the E1 b0-unloaded-average.csv" >&2; exit 2; }
if [[ -z "$worker_cores" ]]; then
    worker_cores=${WORKER_CORES:-$(reference_unique_value worker_cores)}
fi
for workload in "${workloads[@]}"; do reference_value "$workload" rref >/dev/null; done
if [[ "$dry_run" != true ]]; then validate_claim_sources; fi

for ((repetition=0; repetition<repetitions; repetition++)); do
    read -r -a rotated_python <<< "$(rotate python_modes "$repetition")"
    read -r -a rotated_workloads <<< "$(rotate workloads "$repetition")"
    for workload in "${rotated_workloads[@]}"; do
        for mode in "${rotated_python[@]}"; do
            rps=$(reference_value "$workload" rref)
            destination="$result_root/rep-$repetition/$mode/$workload"
            if [[ "$dry_run" == true ]]; then
                print_cell collection "$repetition" "$mode" "$workload" "$rps" "$replicas" true "$warmup_minutes" "$measurement_minutes" "$destination"
            else
                run_cell collection "$repetition" "$mode" "$workload" "$rps" true "$measurement_minutes" "$destination"
            fi
        done
    done
    read -r -a rotated_extra <<< "$(rotate hello_extra_modes "$repetition")"
    hello_rps=$(reference_value helloworld rref)
    for mode in "${rotated_extra[@]}"; do
        destination="$result_root/rep-$repetition/$mode/helloworld"
        if [[ "$dry_run" == true ]]; then
            print_cell collection "$repetition" "$mode" helloworld "$hello_rps" "$replicas" true "$warmup_minutes" "$measurement_minutes" "$destination"
        else
            run_cell collection "$repetition" "$mode" helloworld "$hello_rps" true "$measurement_minutes" "$destination"
        fi
    done
done
if [[ "$dry_run" == true ]]; then
    echo "E2_DRY_RUN_READY result_root=$result_root"
else
    echo "E2_COLLECTION_READY result_root=$result_root"
fi
