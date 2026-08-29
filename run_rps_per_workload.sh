#!/usr/bin/env bash
# E2 is launched from a non-login shell; cluster setup adds the pinned Go
# toolchain to PATH through /etc/profile.
source /etc/profile
set -euo pipefail
source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)/scripts/util/cell_lifecycle.sh"

repo_root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)
cd "$repo_root"
scratch_root=${EVAL_SCRATCH_ROOT:-/mnt/resources/nexus-evaluation/.scratch/e2}
[[ "$scratch_root" == /* && "$scratch_root" != "/" ]] || {
    echo "EVAL_SCRATCH_ROOT must be a non-root absolute path" >&2
    exit 2
}
scratch_root=$(realpath -m -- "$scratch_root")
case "$scratch_root" in
    "$repo_root"|"$repo_root"/*)
        echo "EVAL_SCRATCH_ROOT must be outside the InVitro worktree" >&2
        exit 2
        ;;
esac

command=${1:-}
if [[ -n "$command" ]]; then shift; fi
profile=4-node
e1_summary=
reference=
worker_cores=
slo_multiplier=5
failure_threshold=0.05
ceiling_multiplier=1
warmup_minutes=2
steps=20
minutes_per_step=1
measurement_minutes=3
replicas=320
repetitions=1
result_root=
minio_endpoint=myminio-api.minio.10.200.3.4.sslip.io:80
dry_run=false
no_retry=false
smoke=false
eval_firecracker_head=${EVAL_FIRECRACKER_HEAD:-}
eval_firecracker_branch=${EVAL_FIRECRACKER_BRANCH:-}
eval_rdma_demo_head=${EVAL_RDMA_DEMO_HEAD:-}
eval_rdma_demo_branch=${EVAL_RDMA_DEMO_BRANCH:-}

usage() {
    cat <<'EOF'
Usage:
  run_rps_per_workload.sh calibrate --profile 4-node --e1-summary FILE --worker-cores N
      --slo-multiplier 5 --failure-threshold 0.05 --warmup-minutes 2 --steps 20
      --minutes-per-step 1 --ceiling-multiplier 1 --no-retry --result-root PATH [--dry-run]
  run_rps_per_workload.sh collect --profile 4-node --reference FILE --replicas 320
      --repetitions 1 --result-root PATH [--dry-run]
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
        --ceiling-multiplier) ceiling_multiplier=${2:?}; shift 2 ;;
        --warmup-minutes) warmup_minutes=${2:?}; shift 2 ;;
        --steps) steps=${2:?}; shift 2 ;;
        --minutes-per-step) minutes_per_step=${2:?}; shift 2 ;;
        --measurement-minutes) measurement_minutes=${2:?}; shift 2 ;;
        --replicas) replicas=${2:?}; shift 2 ;;
        --repetitions) repetitions=${2:?}; shift 2 ;;
        --result-root) result_root=${2:?}; shift 2 ;;
        --minio-endpoint) minio_endpoint=${2:?}; shift 2 ;;
        --no-retry) no_retry=true; shift ;;
        --smoke) smoke=true; shift ;;
        --dry-run) dry_run=true; shift ;;
        -h|--help) usage; exit 0 ;;
        *) echo "unknown option: $1" >&2; usage >&2; exit 2 ;;
    esac
done

[[ "$command" == calibrate || "$command" == collect ]] || { usage >&2; exit 2; }
[[ "$profile" == 4-node ]] || { echo "E2 supports only the frozen 4-node profile" >&2; exit 2; }
[[ -n "$result_root" ]] || { echo "--result-root is required" >&2; exit 2; }
[[ "$warmup_minutes" == 2 ]] || { echo "E2 requires --warmup-minutes 2" >&2; exit 2; }
[[ "$minio_endpoint" == myminio-api.minio.10.200.3.4.sslip.io:80 ]] || {
    echo "E2 requires the Kubernetes MinIO ingress myminio-api.minio.10.200.3.4.sslip.io:80" >&2; exit 2; }

workloads=(helloworld chameleonserve cnnserve imageresize lrserving mapper pyaesserve reducer rnnserve streducer sttrainer)
python_modes=(invm-py nexus-py nexus-rdma-py)
hello_extra_modes=(invm-go invm-js hosttcp-go nexus-go nexus-js)
if [[ "$smoke" == true ]]; then
    workloads=(helloworld)
    python_modes=(invm-py)
    hello_extra_modes=(nexus-py)
fi

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
if row.get('status') not in ('BOUNDARY_OBSERVED', 'RIGHT_CENSORED') or not row[column]:
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

repo_value() {
    local field=$1 path=$2 override=${3:-}
    if [[ -d "$path/.git" ]]; then
        case "$field" in
            head) git -C "$path" rev-parse HEAD ;;
            branch) git -C "$path" branch --show-current ;;
            status) git -C "$path" status --short | tr '\n' '|' ;;
        esac
    else
        [[ -n "$override" ]] || { echo "missing provenance for $path/$field" >&2; return 1; }
        printf '%s\n' "$override"
    fi
}

validate_claim_sources() {
    require_clean_repo . invitro
    require_clean_repo ../khala khala
    if [[ -d ../firecracker/.git ]]; then require_clean_repo ../firecracker firecracker
    elif [[ -z "$eval_firecracker_head" || -z "$eval_firecracker_branch" ]]; then
        echo "missing frozen Firecracker source provenance" >&2; exit 2
    fi
    if [[ -d ../rdma-demo/.git ]]; then require_clean_repo ../rdma-demo rdma-demo
    elif [[ -z "$eval_rdma_demo_head" || -z "$eval_rdma_demo_branch" ]]; then
        echo "missing frozen RDMA source provenance" >&2; exit 2
    fi
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

digest() { sha256sum "$1" | awk '{print $1}'; }

line_is() { grep -Fqx "$2" "$1"; }

snapshot_cleanup_policy_matches() {
    grep -Eq '^snapshot_cleanup_policy=pre-cell=(preserve|invalidate-stale-scratch);(recovery-[12]=invalidate;)*final=preserve$' "$1"
}

mode_vm_config() {
    case "$1" in
        invm-js|nexus-js) printf '%s\n' configs/vm_orchestrator_config_js.json ;;
        *) printf '%s\n' configs/vm_orchestrator_config.json ;;
    esac
}

config_value() {
    python3 - "$1" "$2" <<'PY'
import json, sys
with open(sys.argv[1], encoding="utf-8") as handle:
    value = json.load(handle)[sys.argv[2]]
if not isinstance(value, str) or not value:
    raise SystemExit(f"missing {sys.argv[2]} in {sys.argv[1]}")
print(value)
PY
}

khala_artifact_hash() { digest "../khala/$1"; }

tracked_workload_sha() {
    (cd ../khala && git ls-files workload | LC_ALL=C sort | while IFS= read -r path; do sha256sum "$path"; done | sha256sum | awk '{print $1}')
}

discover_cluster_topology() {
    local inventory_path=$1 worker_config_path=$2 master_count loader_count worker_count tenant_count node_count workers_csv tenants_csv loaders_csv
    kubectl get nodes -o json | jq -S '[.items[] | {
        name: .metadata.name,
        internal_ip: ([.status.addresses[]? | select(.type == "InternalIP") | .address] | first),
        loader_nodetype: (.metadata.labels["loader-nodetype"] // ""),
        minio_type: (.metadata.labels["minio-type"] // ""),
        kubelet_version: .status.nodeInfo.kubeletVersion,
        kernel_version: .status.nodeInfo.kernelVersion,
        os_image: .status.nodeInfo.osImage,
        container_runtime_version: .status.nodeInfo.containerRuntimeVersion
    }] | sort_by(.name)' > "$inventory_path"
    master_count=$(kubectl get nodes -l loader-nodetype=master --no-headers 2>/dev/null | wc -l)
    loader_count=$(kubectl get nodes -l loader-nodetype=monitoring --no-headers 2>/dev/null | wc -l)
    worker_count=$(kubectl get nodes -l loader-nodetype=worker --no-headers 2>/dev/null | wc -l)
    tenant_count=$(kubectl get nodes -l minio-type=tenant --no-headers 2>/dev/null | wc -l)
    node_count=$(kubectl get nodes --no-headers | wc -l)
    [[ "$master_count" == 1 && "$loader_count" == 1 && "$worker_count" == 1 && "$tenant_count" == 1 && "$node_count" == 4 ]] || {
        echo "live labels do not match frozen E2 4-node profile: master=$master_count loader=$loader_count worker=$worker_count tenant=$tenant_count total=$node_count" >&2; return 2; }
    mapfile -t workers < <(kubectl get nodes -l loader-nodetype=worker -o jsonpath='{range .items[*]}{.status.addresses[?(@.type=="InternalIP")].address}{"\n"}{end}' | LC_ALL=C sort)
    mapfile -t tenants < <(kubectl get nodes -l minio-type=tenant -o jsonpath='{range .items[*]}{.status.addresses[?(@.type=="InternalIP")].address}{"\n"}{end}' | LC_ALL=C sort)
    mapfile -t loaders < <(kubectl get nodes -l loader-nodetype=monitoring -o jsonpath='{range .items[*]}{.status.addresses[?(@.type=="InternalIP")].address}{"\n"}{end}' | LC_ALL=C sort)
    workers_csv=$(IFS=,; echo "${workers[*]}"); tenants_csv=$(IFS=,; echo "${tenants[*]}"); loaders_csv=$(IFS=,; echo "${loaders[*]}")
    jq -n --arg workers "$workers_csv" --arg storage "$tenants_csv" --arg loaders "$loaders_csv" \
        '{worker_nodes:($workers|split(",")),storage_nodes:($storage|split(",")),loader_nodes:($loaders|split(","))}' > "$worker_config_path"
}

snapshot_remote_provenance() {
    local output=$1 worker_config=$2 require_rdma=$3 expected_head expected_invitro_head expected_workload host vm_config rootfs kernel vmm
    local flamegraph_repo flamegraph_commit
    expected_head=$(git -C ../khala rev-parse HEAD)
    expected_invitro_head=$(git rev-parse HEAD)
    expected_workload=$(tracked_workload_sha)
    flamegraph_repo=$(config_value scripts/setup/configs/setup.json FLAMEGRAPH_REPO)
    flamegraph_commit=$(config_value scripts/setup/configs/setup.json FLAMEGRAPH_COMMIT)
    : > "$output"
    mapfile -t provenance_workers < <(jq -r '.worker_nodes[]' "$worker_config" | LC_ALL=C sort)
    mapfile -t provenance_loaders < <(jq -r '.loader_nodes[]' "$worker_config" | LC_ALL=C sort)
    ((${#provenance_workers[@]} == 1 && ${#provenance_loaders[@]} == 1)) || {
        echo "E2 expected exactly one label-discovered worker and loader node" >&2; return 2; }
    if [[ "$require_rdma" == true ]]; then
        mapfile -t provenance_storage < <(jq -r '.storage_nodes[]' "$worker_config" | LC_ALL=C sort)
        ((${#provenance_storage[@]} == 1)) || { echo "E2 collection expected one label-discovered RDMA storage node" >&2; return 2; }
    fi
    for host in "${provenance_workers[@]}"; do
        ssh -o BatchMode=yes -o StrictHostKeyChecking=accept-new -o ConnectTimeout=5 "$host" bash -s -- "$host" "$flamegraph_repo" "$flamegraph_commit" <<'SH' >> "$output"
set -euo pipefail
host=$1 expected_repo=$2 expected_commit=$3
head=$(git -C ~/FlameGraph rev-parse HEAD)
origin=$(git -C ~/FlameGraph config --get remote.origin.url)
status=$(git -C ~/FlameGraph status --porcelain)
test "$head" = "$expected_commit"
test "$origin" = "$expected_repo"
test -z "$status"
test -x ~/FlameGraph/stackcollapse-perf.pl
test -x ~/FlameGraph/flamegraph.pl
collapse_sha=$(sha256sum ~/FlameGraph/stackcollapse-perf.pl | awk '{print $1}')
flamegraph_sha=$(sha256sum ~/FlameGraph/flamegraph.pl | awk '{print $1}')
printf 'role=worker host=%s tree=flamegraph head=%s stackcollapse_sha256=%s flamegraph_sha256=%s status=clean\n' "$host" "$head" "$collapse_sha" "$flamegraph_sha"
SH
        for vm_config in configs/vm_orchestrator_config.json configs/vm_orchestrator_config_js.json; do
            rootfs=$(config_value "../khala/$vm_config" RootfsPath)
            kernel=$(config_value "../khala/$vm_config" KernelPath)
            vmm=$(config_value "../khala/$vm_config" FirecrackerPath)
            ssh -o BatchMode=yes -o StrictHostKeyChecking=accept-new -o ConnectTimeout=5 "$host" bash -s -- "$host" "$vm_config" "$rootfs" "$kernel" "$vmm" \
                "$expected_head" "$(khala_artifact_hash "$vm_config")" "$(khala_artifact_hash "$rootfs")" \
                "$(khala_artifact_hash "$kernel")" "$(khala_artifact_hash "$vmm")" "$(khala_artifact_hash bin/kn-integration)" "$(khala_artifact_hash bin/nexus-backend)" "$(khala_artifact_hash bin/hardware-manager)" "$expected_workload" <<'SH' >> "$output"
set -euo pipefail
host=$1 vm_config=$2 rootfs=$3 kernel=$4 vmm=$5 expected_head=$6 expected_config=$7 expected_rootfs=$8 expected_kernel=$9 expected_vmm=${10} expected_binary=${11} expected_nexus_backend=${12} expected_hardware_manager=${13} expected_workload=${14}
cd ~/khala
head=$(git rev-parse HEAD); status=$(git status --porcelain)
[[ "$head" == "$expected_head" && -z "$status" ]]
workload=$(git ls-files workload | LC_ALL=C sort | while IFS= read -r path; do sha256sum "$path"; done | sha256sum | awk '{print $1}')
[[ "$workload" == "$expected_workload" ]]
for item in "$vm_config:$expected_config" "$rootfs:$expected_rootfs" "$kernel:$expected_kernel" "$vmm:$expected_vmm" "bin/kn-integration:$expected_binary" "bin/nexus-backend:$expected_nexus_backend" "bin/hardware-manager:$expected_hardware_manager"; do
    path=${item%%:*}; expected=${item#*:}; actual=$(sha256sum "$path" | awk '{print $1}'); [[ "$actual" == "$expected" ]]; printf 'role=worker host=%s tree=khala path=%s sha256=%s\n' "$host" "$path" "$actual"
done
printf 'role=worker host=%s tree=khala head=%s workload_sha256=%s status=clean\n' "$host" "$head" "$workload"
SH
        done
    done
    for host in "${provenance_loaders[@]}"; do
        ssh -o BatchMode=yes -o StrictHostKeyChecking=accept-new -o ConnectTimeout=5 "$host" bash -s -- "$host" "$expected_invitro_head" <<'SH' >> "$output"
set -euo pipefail
host=$1 expected_head=$2; cd ~/loader; head=$(git rev-parse HEAD)
test "$head" = "$expected_head"; test -z "$(git status --porcelain)"
printf 'role=loader host=%s tree=loader head=%s expected_head=%s status=clean\n' "$host" "$head" "$expected_head"
SH
    done
    if [[ "$require_rdma" == true ]]; then
        for host in "${provenance_storage[@]}"; do
            ssh -o BatchMode=yes -o StrictHostKeyChecking=accept-new -o ConnectTimeout=5 "$host" bash -s -- "$host" "$eval_rdma_demo_head" <<'SH' >> "$output"
set -euo pipefail
host=$1 expected_head=$2; cd ~/rdma-demo
head=$(git rev-parse HEAD); status=$(git status --porcelain); binary=$(sha256sum s3-rdma-server | awk '{print $1}')
[[ "$head" == "$expected_head" && -z "$status" ]]
printf 'role=storage host=%s tree=rdma-demo head=%s path=s3-rdma-server sha256=%s status=clean\n' "$host" "$head" "$binary"
SH
        done
    fi
    LC_ALL=C sort -o "$output" "$output"
    [[ -s "$output" ]]
}

write_archived_output_checksums() {
    local directory=$1 checksum_file=archived-output-checksums.csv
    (
        cd "$directory"
        printf 'path,sha256\n' > "$checksum_file"
        while IFS= read -r -d '' path; do
            printf '%s,%s\n' "$path" "$(sha256sum "$path" | awk '{print $1}')" >> "$checksum_file"
        done < <(find . -type f ! -name manifest.txt ! -name "$checksum_file" -printf '%P\0' | LC_ALL=C sort -z)
    )
}

archived_output_matches() {
    lifecycle_archived_output_matches "$1"
}

manifest_matches() {
    local manifest=$1 phase=$2 repetition=$3 mode=$4 workload=$5 rps=$6 perf=$7 duration=$8 destination=$9
    local vm_config rootfs kernel vmm worker_count expected_perf_artifacts
    [[ -f "$manifest" ]] || return 1
    vm_config=$(mode_vm_config "$mode")
    rootfs=$(config_value "../khala/$vm_config" RootfsPath)
    kernel=$(config_value "../khala/$vm_config" KernelPath)
    vmm=$(config_value "../khala/$vm_config" FirecrackerPath)
        line_is "$manifest" 'manifest_version=2' &&
        line_is "$manifest" "smoke=$smoke" &&
        line_is "$manifest" "phase=$phase" && line_is "$manifest" "repetition=$repetition" &&
        line_is "$manifest" "mode=$mode" && line_is "$manifest" "workload=$workload" &&
        line_is "$manifest" "rps=$rps" && line_is "$manifest" "replicas=$replicas" &&
        line_is "$manifest" "perf_enabled=$perf" && line_is "$manifest" "profile=$profile" &&
        line_is "$manifest" "minio_endpoint=$minio_endpoint" &&
        line_is "$manifest" "warmup_minutes=$warmup_minutes" && line_is "$manifest" "measurement_minutes=$duration" &&
        line_is "$manifest" 'scan_snapshot=false' &&
        line_is "$manifest" "ceiling_multiplier=$ceiling_multiplier" &&
        line_is "$manifest" "invitro_head=$(git rev-parse HEAD)" &&
        line_is "$manifest" "khala_head=$(git -C ../khala rev-parse HEAD)" &&
        line_is "$manifest" "firecracker_head=$(repo_value head ../firecracker "$eval_firecracker_head")" &&
        line_is "$manifest" "rdma_demo_head=$(repo_value head ../rdma-demo "$eval_rdma_demo_head")" &&
        line_is "$manifest" "e1_summary_sha256=$(digest "$e1_summary")" &&
        line_is "$manifest" "calibrator_sha256=$(digest e2_calibrate_rps.py)" &&
        line_is "$manifest" "runner_sha256=$(digest run_rps_per_workload.sh)" &&
        line_is "$manifest" "evidence_validator_sha256=$(digest experiment/e2/validate_evidence.py)" &&
        line_is "$manifest" "config_template_sha256=$(digest cmd/config_khala_trace_template.json)" &&
        line_is "$manifest" "vm_config_path=$vm_config" &&
        line_is "$manifest" "vm_config_sha256=$(khala_artifact_hash "$vm_config")" &&
        line_is "$manifest" "rootfs_path=$rootfs" && line_is "$manifest" "rootfs_sha256=$(khala_artifact_hash "$rootfs")" &&
        line_is "$manifest" "kernel_path=$kernel" && line_is "$manifest" "kernel_sha256=$(khala_artifact_hash "$kernel")" &&
        line_is "$manifest" "vmm_path=$vmm" && line_is "$manifest" "vmm_sha256=$(khala_artifact_hash "$vmm")" &&
        line_is "$manifest" "workload_sha256=$(tracked_workload_sha)" &&
        line_is "$manifest" "remote_provenance_sha256=$(digest "$result_root/remote-provenance.txt")" &&
        line_is "$manifest" "cluster_inventory_sha256=$(digest "$result_root/cluster-inventory.txt")" &&
        line_is "$manifest" "worker_config_sha256=$(digest "$result_root/worker-node.json")" &&
        line_is "$manifest" 'evidence_status=0' &&
        line_is "$manifest" 'exit_status=0' || return 1
    lifecycle_success_manifest_matches "$manifest" || return 1
    snapshot_cleanup_policy_matches "$manifest" || return 1
    worker_count=$(jq '.worker_nodes | length' "$destination/worker-node.json")
    expected_perf_artifacts=0
    if [[ "$perf" == true ]]; then expected_perf_artifacts=$((worker_count * 4)); fi
    line_is "$manifest" "perf_artifact_count=$expected_perf_artifacts" || return 1
    line_is "$manifest" "evidence_validation_sha256=$(digest "$destination/evidence-validation.txt")" || return 1
    if [[ "$phase" == collection ]]; then
        line_is "$manifest" "reference_sha256=$(digest "$reference")" || return 1
        cmp --silent "$reference" "$result_root/b0-rps-reference.csv" || return 1
    fi
    cmp --silent "$destination/remote-provenance.txt" "$result_root/remote-provenance.txt" || return 1
    cmp --silent "$destination/cluster-inventory.txt" "$result_root/cluster-inventory.txt" || return 1
    cmp --silent "$destination/worker-node.json" "$result_root/worker-node.json" || return 1
    cmp --silent "$destination/e1-b0-unloaded-average.csv" "$e1_summary" || return 1
    if [[ "$phase" == collection ]]; then
        cmp --silent "$destination/b0-rps-reference.csv" "$reference" || return 1
    fi
    archived_output_matches "$destination"
}

prepare_cluster_root() {
    local require_reference=$1 check_dir
    if [[ -e "$result_root" ]]; then
        [[ -f "$result_root/cluster-inventory.txt" && -f "$result_root/worker-node.json" && -f "$result_root/remote-provenance.txt" && -f "$result_root/e1-b0-unloaded-average.csv" ]] || {
            echo "existing E2 result root lacks archived cluster provenance" >&2; return 2; }
        cmp --silent "$e1_summary" "$result_root/e1-b0-unloaded-average.csv" || {
            echo "archived E1 reference differs from this run" >&2; return 2; }
        if [[ "$require_reference" == true ]]; then
            [[ -f "$result_root/b0-rps-reference.csv" ]] && cmp --silent "$reference" "$result_root/b0-rps-reference.csv" || {
                echo "archived B0 reference differs from collection input" >&2; return 2; }
        fi
        check_dir=$(mktemp -d)
        discover_cluster_topology "$check_dir/cluster-inventory.txt" "$check_dir/worker-node.json"
        cmp --silent "$check_dir/cluster-inventory.txt" "$result_root/cluster-inventory.txt" &&
            cmp --silent "$check_dir/worker-node.json" "$result_root/worker-node.json" || {
                rm -r -- "$check_dir"; echo "live E2 topology differs from archived result root" >&2; return 2; }
        snapshot_remote_provenance "$check_dir/remote-provenance.txt" "$check_dir/worker-node.json" "$require_reference"
        cmp --silent "$check_dir/remote-provenance.txt" "$result_root/remote-provenance.txt" || {
            rm -r -- "$check_dir"; echo "remote provenance differs from archived result root" >&2; return 2; }
        rm -r -- "$check_dir"
    else
        mkdir -p "$result_root"
        cp -- "$e1_summary" "$result_root/e1-b0-unloaded-average.csv"
        if [[ "$require_reference" == true ]]; then cp -- "$reference" "$result_root/b0-rps-reference.csv"; fi
        discover_cluster_topology "$result_root/cluster-inventory.txt" "$result_root/worker-node.json"
        snapshot_remote_provenance "$result_root/remote-provenance.txt" "$result_root/worker-node.json" "$require_reference"
    fi
}

initial_cleanup_matches() {
    local destination=$1 manifest="$destination/manifest.txt"
    [[ -f "$manifest" ]] || return 1
    line_is "$manifest" 'manifest_version=1' &&
        line_is "$manifest" 'initial_cleanup=true' &&
        line_is "$manifest" 'cleanup_mode=nexus-rdma-py' &&
        line_is "$manifest" 'remove_snapshots=true' &&
        line_is "$manifest" "worker_config_sha256=$(digest "$result_root/worker-node.json")" &&
        line_is "$manifest" "cluster_inventory_sha256=$(digest "$result_root/cluster-inventory.txt")" &&
        line_is "$manifest" "remote_provenance_sha256=$(digest "$result_root/remote-provenance.txt")" &&
        line_is "$manifest" "runner_sha256=$(digest run_rps_per_workload.sh)" &&
        line_is "$manifest" 'exit_status=0' || return 1
    archived_output_matches "$destination"
}

run_initial_cleanup() {
    local destination="$result_root/initial-cleanup" scratch status started
    if [[ -e "$destination" ]]; then
        initial_cleanup_matches "$destination" || {
            echo "initial cleanup evidence is incomplete or tampered: $destination" >&2
            return 2
        }
        echo "RESUME verified initial cleanup"
        return 0
    fi
    mkdir -p "$scratch_root"
    scratch=$(mktemp -d "$scratch_root/initial-cleanup.XXXXXX")
    started=$(date -u --iso-8601=seconds)
    set +e
    go run experiment/khala_command.go --command clean --mode nexus-rdma-py --worker-config "$result_root/worker-node.json" \
        --minio-endpoint "$minio_endpoint" --remove-snapshots=true > "$scratch/clean.log" 2>&1
    status=$?
    set -e
    {
        echo manifest_version=1
        echo initial_cleanup=true
        echo cleanup_mode=nexus-rdma-py
        echo remove_snapshots=true
        echo "start_utc=$started"
        echo "end_utc=$(date -u --iso-8601=seconds)"
        echo "worker_config_sha256=$(digest "$result_root/worker-node.json")"
        echo "cluster_inventory_sha256=$(digest "$result_root/cluster-inventory.txt")"
        echo "remote_provenance_sha256=$(digest "$result_root/remote-provenance.txt")"
        echo "runner_sha256=$(digest run_rps_per_workload.sh)"
        echo "exit_status=$status"
    } > "$scratch/manifest.txt"
    mkdir -- "$destination" || { rm -rf -- "$scratch"; return 2; }
    cp -a -- "$scratch/." "$destination/"
    write_archived_output_checksums "$destination"
    rm -rf -- "$scratch"
    archived_output_matches "$destination" || { echo "initial cleanup archive verification failed" >&2; return 2; }
    ((status == 0)) || return "$status"
    initial_cleanup_matches "$destination"
}

run_cell() {
    local phase=$1 repetition=$2 mode=$3 workload=$4 rps=$5 perf=$6 duration=$7 destination=$8 worker_config=$9
    local run_id="e2-${phase}-r${repetition}-${mode}-${workload}"
    local scratch_cell="$scratch_root/$run_id"
    local scratch_trace="$scratch_cell/trace"
    local scratch_out="$scratch_cell/out"
    local config_path="$scratch_out/config.json"
    local manifest="$destination/manifest.txt"
    if [[ -e "$destination" ]] && manifest_matches "$manifest" "$phase" "$repetition" "$mode" "$workload" "$rps" "$perf" "$duration" "$destination"; then
        echo "RESUME skip $run_id"
        return
    fi
    [[ ! -e "$destination" ]] || { echo "refusing incomplete cell: $destination" >&2; return 2; }
    local evidence_status=1 perf_artifact_count=0 stale_scratch=false snapshot_cleanup_policy=
    if [[ -e "$scratch_cell" ]]; then stale_scratch=true; fi
    lifecycle_setup() {
        local attempt=$1
        if ((attempt == 1)); then
            # Keep the pre-cell cleanup log as part of the eventual archive;
            # discard only stale setup/acquisition artifacts.
            if [[ -d "$scratch_cell" ]]; then
                find "$scratch_cell" -mindepth 1 -maxdepth 1 ! -name out -exec rm -rf -- {} +
            fi
            if [[ -d "$scratch_out" ]]; then
                find "$scratch_out" -mindepth 1 -maxdepth 1 ! -name clean-pre-cell.log -exec rm -rf -- {} +
            fi
            mkdir -p "$scratch_out"
        else
            local previous_attempt=$((attempt - 1)) preserved="$scratch_out/setup-attempt-$previous_attempt"
            mkdir -p "$preserved"
            find "$scratch_out" -mindepth 1 -maxdepth 1 ! -name 'setup-attempt-*' -exec cp -a -- {} "$preserved" \;
            rm -rf -- "$scratch_trace" "$scratch_out/trace"
            rm -f -- "$scratch_out/trace-generator.log" "$config_path" "$scratch_out/manifest.txt"
        fi
        if [[ "$phase" == calibration ]]; then
            python3 e2_calibrate_rps.py --averages "$e1_summary" --cores "$worker_cores" --ceiling-multiplier "$ceiling_multiplier" trace \
                --workload "$workload" --warmup-minutes "$warmup_minutes" --output "$scratch_trace"
        else
            python3 e2_calibrate_rps.py --averages "$e1_summary" --cores "$worker_cores" --ceiling-multiplier "$ceiling_multiplier" fixed-trace \
                --workload "$workload" --mode "$mode" --rps "$rps" --warmup-minutes "$warmup_minutes" \
                --measurement-minutes "$duration" --output "$scratch_trace"
        fi
        write_config "$run_id" "$duration" "$perf" "$replicas" "$scratch_trace" "$scratch_out/experiment" "$config_path"
        cp -a -- "$scratch_trace" "$scratch_out/trace"
        cp -- "$result_root/remote-provenance.txt" "$scratch_out/remote-provenance.txt"
        cp -- "$result_root/cluster-inventory.txt" "$scratch_out/cluster-inventory.txt"
        cp -- "$worker_config" "$scratch_out/worker-node.json"
        cp -- "$e1_summary" "$scratch_out/e1-b0-unloaded-average.csv"
        if [[ "$phase" == collection ]]; then cp -- "$reference" "$scratch_out/b0-rps-reference.csv"; fi
        {
        vm_config=$(mode_vm_config "$mode")
        rootfs=$(config_value "../khala/$vm_config" RootfsPath)
        kernel=$(config_value "../khala/$vm_config" KernelPath)
        vmm=$(config_value "../khala/$vm_config" FirecrackerPath)
        echo manifest_version=2
        echo "smoke=$smoke"
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
        echo "firecracker_head=$(repo_value head ../firecracker "$eval_firecracker_head")"
        echo "firecracker_branch=$(repo_value branch ../firecracker "$eval_firecracker_branch")"
        echo "firecracker_status=$(repo_value status ../firecracker clean)"
        echo "rdma_demo_head=$(repo_value head ../rdma-demo "$eval_rdma_demo_head")"
        echo "rdma_demo_branch=$(repo_value branch ../rdma-demo "$eval_rdma_demo_branch")"
        echo "rdma_demo_status=$(repo_value status ../rdma-demo clean)"
        echo "profile=$profile"
        echo "minio_endpoint=$minio_endpoint"
        echo "warmup_minutes=$warmup_minutes"
        echo "measurement_minutes=$duration"
        echo "scan_snapshot=false"
        echo "slo_multiplier=$slo_multiplier"
        echo "failure_threshold=$failure_threshold"
        echo "ceiling_multiplier=$ceiling_multiplier"
        echo "worker_cores=$worker_cores"
        echo "e1_summary_sha256=$(digest "$e1_summary")"
        echo "calibrator_sha256=$(digest e2_calibrate_rps.py)"
        echo "runner_sha256=$(digest run_rps_per_workload.sh)"
        echo "evidence_validator_sha256=$(digest experiment/e2/validate_evidence.py)"
        echo "config_template_sha256=$(digest cmd/config_khala_trace_template.json)"
        echo "trace_modes_sha256=$(digest trace_modes.py)"
        echo "trace_invocations_sha256=$(digest "$scratch_trace/invocations.csv")"
        echo "trace_durations_sha256=$(digest "$scratch_trace/durations.csv")"
        echo "config_sha256=$(digest "$config_path")"
        echo "vm_config_path=$vm_config"
        echo "vm_config_sha256=$(khala_artifact_hash "$vm_config")"
        echo "rootfs_path=$rootfs"
        echo "rootfs_sha256=$(khala_artifact_hash "$rootfs")"
        echo "kernel_path=$kernel"
        echo "kernel_sha256=$(khala_artifact_hash "$kernel")"
        echo "vmm_path=$vmm"
        echo "vmm_sha256=$(khala_artifact_hash "$vmm")"
        echo "workload_sha256=$(tracked_workload_sha)"
        echo "remote_provenance_sha256=$(digest "$result_root/remote-provenance.txt")"
        echo "cluster_inventory_sha256=$(digest "$result_root/cluster-inventory.txt")"
        echo "worker_config_sha256=$(digest "$worker_config")"
        if [[ -f "$reference" ]]; then echo "reference_sha256=$(digest "$reference")"; fi
        } > "$scratch_out/manifest.txt"
    }
    lifecycle_deploy() {
        local attempt=$1
        go run experiment/khala_command.go --command deploy --mode "$mode" --worker-config "$worker_config" --workloads "$workload" \
            --shmem-ring-bytes 4190208 --shmem-io-quantum 262144 --minio-endpoint "$minio_endpoint" \
            2>&1 | tee "$scratch_out/deploy-attempt-$attempt.log"
        local status=${PIPESTATUS[0]}
        cat "$scratch_out/deploy-attempt-$attempt.log" >> "$scratch_out/deploy.log"
        return "$status"
    }
    lifecycle_run() {
        go run cmd/loader.go --config "$config_path" > >(tee "$scratch_out/loader.log") 2>&1
        local status=$?
        python3 experiment/e2/validate_evidence.py --output-prefix "$scratch_out/experiment" \
            --loader-log "$scratch_out/loader.log" --worker-config "$worker_config" --perf-enabled "$perf" \
            > "$scratch_out/evidence-validation.txt" 2>&1
        evidence_status=$?
        perf_artifact_count=$(awk -F= '$1 == "perf_artifact_count" {print $2}' "$scratch_out/evidence-validation.txt")
        perf_artifact_count=${perf_artifact_count:-0}
        if ((status == 0 && evidence_status != 0)); then status=$evidence_status; fi
        return "$status"
    }
    lifecycle_cleanup() {
        local cleanup_phase=$1 policy remove_snapshots=false
        case "$cleanup_phase" in
            pre-cell)
                if [[ "$stale_scratch" == true ]]; then
                    policy=invalidate-stale-scratch
                    remove_snapshots=true
                else
                    policy=preserve
                fi
                ;;
            recovery-1|recovery-2) policy=invalidate; remove_snapshots=true ;;
            final) policy=preserve ;;
            *) echo "unknown lifecycle cleanup phase: $cleanup_phase" >&2; return 2 ;;
        esac
        if [[ -n "$snapshot_cleanup_policy" ]]; then snapshot_cleanup_policy+=';'; fi
        snapshot_cleanup_policy+="$cleanup_phase=$policy"
        go run experiment/khala_command.go --command clean --mode "$mode" --worker-config "$worker_config" --minio-endpoint "$minio_endpoint" \
            --remove-snapshots="$remove_snapshots" > "$scratch_out/clean-$cleanup_phase.log" 2>&1
        local status=$?
        cat "$scratch_out/clean-$cleanup_phase.log" >> "$scratch_out/clean.log"
        return "$status"
    }
    lifecycle_finalize() {
        local status=$1 clean_status=$2 setup_attempts=$3 deploy_invocations=$4 loader_started=$5
        if [[ ! -f "$scratch_out/evidence-validation.txt" ]]; then
            printf '%s\n' 'evidence_status=FAIL reason=loader did not run' > "$scratch_out/evidence-validation.txt"
        fi
        {
            echo "evidence_status=$evidence_status"
            echo "perf_artifact_count=$perf_artifact_count"
            echo "evidence_validation_sha256=$(digest "$scratch_out/evidence-validation.txt")"
            echo "setup_attempts=$setup_attempts"
            echo "deploy_attempts=$deploy_invocations"
            echo "deploy_invocations=$deploy_invocations"
            echo "loader_started=$loader_started"
            echo "cleanup_exit_status=$clean_status"
            echo "snapshot_cleanup_policy=$snapshot_cleanup_policy"
            if [[ "$loader_started" == true ]]; then echo 'lifecycle_phase=final'; else echo 'lifecycle_phase=setup'; fi
        echo "end_utc=$(date -u --iso-8601=seconds)"
        echo "exit_status=$status"
        } >> "$scratch_out/manifest.txt"
        mkdir -p "$(dirname "$destination")"
        cp -a -- "$scratch_out" "$destination"
        write_archived_output_checksums "$destination"
        rm -rf -- "$scratch_cell"
    }
    lifecycle_verify() {
        manifest_matches "$manifest" "$phase" "$repetition" "$mode" "$workload" "$rps" "$perf" "$duration" "$destination"
    }
    mkdir -p "$scratch_out"
    local status
    if lifecycle_preclean; then
        :
    else
        status=$?
        lifecycle_finalize "$status" "$LIFECYCLE_PRECLEAN_STATUS" 0 0 false
        echo "pre-cell cleanup failed; refusing acquisition; immutable evidence retained at $destination" >&2
        return "$status"
    fi
    if lifecycle_execute; then
        return 0
    else
        status=$?
    fi
    if [[ "$LIFECYCLE_CLEANUP_FAILED" == true ]]; then
        echo "cleanup failed; aborting to avoid cross-cell contamination; evidence retained at $destination" >&2
    else
        echo "cell failed; evidence retained at $destination" >&2
    fi
    return "$status"
}

if [[ "$command" == calibrate ]]; then
    [[ -f "$e1_summary" && -n "$worker_cores" ]] || { echo "calibrate requires --e1-summary and --worker-cores" >&2; exit 2; }
    [[ "$slo_multiplier" == 5 && "$failure_threshold" == 0.05 && "$steps" == 20 && "$minutes_per_step" == 1 && "$no_retry" == true ]] || {
        echo "calibration contract is frozen at 5x, >5%, 20 one-minute steps, and no retry" >&2; exit 2; }
    [[ "$ceiling_multiplier" == 1 ]] || {
        echo "the single-pass campaign requires --ceiling-multiplier 1 and never extends a right-censored sweep" >&2; exit 2; }
    plan_path="$result_root/calibration-plan.csv"
    if [[ "$dry_run" == true ]]; then
        python3 - "$e1_summary" "$worker_cores" "$ceiling_multiplier" <<'PY' >/dev/null
import sys
from pathlib import Path
from e2_calibrate_rps import build_plan, read_averages
build_plan(read_averages(Path(sys.argv[1])), int(sys.argv[2]), float(sys.argv[3]))
PY
        for workload in "${workloads[@]}"; do
            print_cell calibration 0 invm-py "$workload" sweep 320 false 2 20 "$result_root/cells/$workload"
        done
        exit 0
    fi
    validate_claim_sources
    prepare_cluster_root false
    scripts/util/wait_prometheus_ready.sh
    run_initial_cleanup || { status=$?; echo "initial cleanup failed; refusing E2 acquisition" >&2; exit "$status"; }
    python3 e2_calibrate_rps.py --averages "$e1_summary" --cores "$worker_cores" --ceiling-multiplier "$ceiling_multiplier" plan --output "$plan_path"
    observations=()
    suite_failed=false
    for workload in "${workloads[@]}"; do
        cell="$result_root/cells/$workload"
        if run_cell calibration 0 invm-py "$workload" sweep false 20 "$cell" "$result_root/worker-node.json"; then
            :
        else
            status=$?
            [[ "$LIFECYCLE_CLEANUP_FAILED" != true ]] || exit "$status"
            suite_failed=true
            continue
        fi
        duration_csv=$(find "$cell" -maxdepth 1 -name 'experiment_duration_*.csv' -print -quit)
        if [[ -z "$duration_csv" ]]; then
            echo "missing duration CSV for $workload" >&2
            suite_failed=true
            continue
        fi
        observation="$cell/observations.csv"
        python3 e2_calibrate_rps.py --averages "$e1_summary" --cores "$worker_cores" --ceiling-multiplier "$ceiling_multiplier" observe \
            --workload "$workload" --duration-csv "$duration_csv" --output "$observation"
        observations+=("$observation")
    done
    if [[ "$suite_failed" == true ]]; then
        echo "E2_CALIBRATION_FAILED result_root=$result_root" >&2
        exit 1
    fi
    python3 e2_calibrate_rps.py --averages "$e1_summary" --cores "$worker_cores" --ceiling-multiplier "$ceiling_multiplier" finalize \
        --observations "${observations[@]}" --output "$result_root/b0-rps-reference.csv"
    if python3 - "$result_root/b0-rps-reference.csv" <<'PY'
import csv, sys
with open(sys.argv[1], newline='', encoding='utf-8') as handle:
    raise SystemExit(0 if any(row['status'] == 'RIGHT_CENSORED' for row in csv.DictReader(handle)) else 1)
PY
    then
        printf '%s\n' 'RIGHT_CENSORED: preserving the single-pass result; rref is a labeled conservative half-highest-tested reference, not 50% of maximum.'
    fi
    echo "CALIBRATION_READY reference=$result_root/b0-rps-reference.csv"
    exit 0
fi

[[ -f "$reference" ]] || { echo "collect requires --reference" >&2; exit 2; }
if [[ "$smoke" == true ]]; then
    [[ "$replicas" == 2 && "$measurement_minutes" == 1 && "$repetitions" == 1 ]] || {
        echo "E2 smoke requires two replicas, one measurement minute, and one pass" >&2; exit 2; }
else
    [[ "$replicas" == 320 && "$repetitions" == 1 ]] || { echo "collection requires 320 replicas and one campaign repetition" >&2; exit 2; }
fi
if [[ -z "$e1_summary" ]]; then
    e1_summary=${E1_SUMMARY:-$(dirname "$reference")/../e1-real/analysis/b0-unloaded-average.csv}
fi
[[ -f "$e1_summary" ]] || { echo "set E1_SUMMARY to the E1 b0-unloaded-average.csv" >&2; exit 2; }
if [[ -z "$worker_cores" ]]; then
    worker_cores=${WORKER_CORES:-$(reference_unique_value worker_cores)}
fi
ceiling_multiplier=$(reference_unique_value ceiling_multiplier)
for workload in "${workloads[@]}"; do reference_value "$workload" rref >/dev/null; done
if [[ "$dry_run" != true ]]; then
    validate_claim_sources
    prepare_cluster_root true
    scripts/util/wait_prometheus_ready.sh
    run_initial_cleanup || { status=$?; echo "initial cleanup failed; refusing E2 acquisition" >&2; exit "$status"; }
fi

suite_failed=false
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
                if run_cell collection "$repetition" "$mode" "$workload" "$rps" true "$measurement_minutes" "$destination" "$result_root/worker-node.json"; then
                    :
                else
                    status=$?
                    [[ "$LIFECYCLE_CLEANUP_FAILED" != true ]] || exit "$status"
                    suite_failed=true
                fi
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
            if run_cell collection "$repetition" "$mode" helloworld "$hello_rps" true "$measurement_minutes" "$destination" "$result_root/worker-node.json"; then
                :
            else
                status=$?
                [[ "$LIFECYCLE_CLEANUP_FAILED" != true ]] || exit "$status"
                suite_failed=true
            fi
        fi
    done
done
if [[ "$dry_run" == true ]]; then
    echo "E2_DRY_RUN_READY result_root=$result_root"
elif [[ "$suite_failed" == true ]]; then
    echo "E2_COLLECTION_FAILED result_root=$result_root" >&2
    exit 1
else
    echo "E2_COLLECTION_READY result_root=$result_root"
fi
