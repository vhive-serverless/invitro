#!/usr/bin/env bash
# E2-Synth is launched from a non-login shell; cluster setup adds the pinned Go
# toolchain to PATH through /etc/profile.
source /etc/profile
set -euo pipefail
source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)/scripts/util/cell_lifecycle.sh"

repo_root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)
cd "$repo_root"
export KHALA_LOCAL_ROOT=${KHALA_LOCAL_ROOT:-$(realpath -- "$repo_root/../khala")}
scratch_root=${EVAL_SCRATCH_ROOT:-/mnt/resources/nexus-evaluation/.scratch/e2-synth}
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
cluster_id=
modes_text=invm-py,invm-js,invm-go,hosttcp-go,nexus-py,nexus-js,nexus-go,nexus-rdma-py,nexus-rdma-go
payloads_text=4,4096,16384,65536,262144,1048576,2097152,4194304,8388608,16777216
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
smoke=false
eval_firecracker_head=${EVAL_FIRECRACKER_HEAD:-}
eval_firecracker_branch=${EVAL_FIRECRACKER_BRANCH:-}
eval_rdma_demo_head=${EVAL_RDMA_DEMO_HEAD:-}
eval_rdma_demo_branch=${EVAL_RDMA_DEMO_BRANCH:-}
readonly expected_loader_branch=jy/khala-asplos-27-e2-synth
readonly expected_khala_branch=jy/asplos-26-e2-synth
readonly vm_shmem_bytes=16777216
readonly shmem_ring_bytes=16773120
readonly shmem_io_quantum=262144

usage() {
    cat <<'EOF'
Usage:
  run_e2_synth.sh calibrate --profile 4-node --cluster-id ID --payloads BYTES,...
      --e1-summary FILE --worker-cores N
      --slo-multiplier 5 --failure-threshold 0.05 --warmup-minutes 2 --steps 20
      --minutes-per-step 1 --ceiling-multiplier 1 --result-root PATH [--dry-run]
  run_e2_synth.sh collect --profile 4-node --cluster-id ID --modes MODE,...
      --payloads BYTES,... --reference FILE --e1-summary FILE --replicas 320
      --warmup-minutes 2 --measurement-minutes 3 --repetitions 1
      --result-root PATH [--smoke] [--dry-run]
EOF
}

while (($#)); do
    case "$1" in
        --profile) profile=${2:?}; shift 2 ;;
        --cluster-id) cluster_id=${2:?}; shift 2 ;;
        --modes) modes_text=${2:?}; shift 2 ;;
        --payloads) payloads_text=${2:?}; shift 2 ;;
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
        --smoke) smoke=true; shift ;;
        --dry-run) dry_run=true; shift ;;
        -h|--help) usage; exit 0 ;;
        *) echo "unknown option: $1" >&2; usage >&2; exit 2 ;;
    esac
done

[[ "$command" == calibrate || "$command" == collect ]] || { usage >&2; exit 2; }
[[ "$profile" == 4-node ]] || { echo "E2-Synth supports only the frozen 4-node profile" >&2; exit 2; }
[[ "$cluster_id" =~ ^[A-Za-z0-9._-]+$ ]] || { echo "--cluster-id is required and must be a stable identifier" >&2; exit 2; }
[[ -n "$result_root" ]] || { echo "--result-root is required" >&2; exit 2; }
for value in "$warmup_minutes" "$measurement_minutes" "$replicas" "$repetitions" "$steps" "$minutes_per_step"; do
    [[ "$value" =~ ^[1-9][0-9]*$ ]] || { echo "E2 counts and durations must be positive integers" >&2; exit 2; }
done
[[ "$minio_endpoint" =~ ^[A-Za-z0-9._-]+:[0-9]+$ ]] || {
    echo "E2 MinIO endpoint must be a non-empty host:port" >&2; exit 2; }

mode_lines=$(python3 - "$modes_text" <<'PY'
import sys
from e2_synth_modes import canonical_mode
seen = set()
for raw in sys.argv[1].split(','):
    mode = canonical_mode(raw)
    if mode in seen:
        raise SystemExit(f'duplicate canonical E2-Synth mode: {mode}')
    seen.add(mode)
    print(mode)
PY
)
mapfile -t modes <<< "$mode_lines"
payload_lines=$(python3 - "$payloads_text" <<'PY'
import sys
from e2_synth_modes import PAYLOADS
seen = set()
for raw in sys.argv[1].split(','):
    try: payload = int(raw)
    except ValueError: raise SystemExit(f'invalid payload: {raw!r}')
    if payload not in PAYLOADS or payload in seen:
        raise SystemExit(f'unsupported or duplicate E2-Synth payload: {payload}')
    seen.add(payload)
    print(payload)
PY
)
mapfile -t payloads <<< "$payload_lines"
((${#modes[@]} > 0 && ${#payloads[@]} > 0)) || { echo "modes and payloads must be non-empty" >&2; exit 2; }
if [[ "$smoke" == true ]]; then
    modes=(invm-py invm-js invm-go hosttcp-go nexus-py nexus-js nexus-go nexus-rdma-py nexus-rdma-go)
    payloads=(65536 16777216)
fi

rotate() {
    local -n values=$1
    local offset=$(( $2 % ${#values[@]} )) index
    for ((index=offset; index<${#values[@]}; index++)); do printf '%s ' "${values[index]}"; done
    for ((index=0; index<offset; index++)); do printf '%s ' "${values[index]}"; done
}

trace_workload_identity() {
    local trace_path=$1 identity
    identity=$(awk -F, '
        NR > 1 && $1 != "" {identities[$1] = 1}
        END {
            if (length(identities) != 1) exit 1
            for (name in identities) print name
        }
    ' "$trace_path") || return 1
    [[ -n "$identity" ]] || return 1
    printf '%s\n' "$identity"
}

reference_value() {
    local payload=$1 column=$2
    python3 - "$reference" "$payload" "$column" <<'PY'
import csv, sys
path, payload, column = sys.argv[1:]
with open(path, newline='', encoding='utf-8') as handle:
    rows = list(csv.DictReader(handle))
matches = [row for row in rows if row.get('payload_bytes', row.get('payload', '')) == payload]
if len(matches) != 1 or column not in matches[0]:
    raise SystemExit(f'missing unique payload {payload}/{column} in {path}')
row = matches[0]
if row.get('status') not in ('BOUNDARY_OBSERVED', 'RIGHT_CENSORED') or not row[column]:
    raise SystemExit(f'payload {payload} is not admissible for collection: status={row.get("status")}')
print(row[column])
PY
}

base_workload() { printf 'synthetic_e_0_p_%s\n' "$1"; }

trace_workload() {
    python3 - "$1" "$2" <<'PY'
import sys
from e2_synth_modes import trace_workload_name
print(trace_workload_name(int(sys.argv[1]), sys.argv[2]))
PY
}

seeded_payload_path() {
    printf '%s/assets/synthetic-payload/object-%s\n' "$KHALA_LOCAL_ROOT" "$1"
}

materialize_selected_payload() {
    local payload=$1 path size
    path=$(seeded_payload_path "$payload")
    SYNTHETIC_PAYLOAD_DIR="$KHALA_LOCAL_ROOT/assets/synthetic-payload" \
        bash "$KHALA_LOCAL_ROOT/scripts/materialize_synthetic_payloads.sh" "$payload"
    [[ -f "$path" && ! -L "$path" ]] || {
        echo "materialized synthetic payload is not a regular file: $path" >&2
        return 1
    }
    size=$(stat -c '%s' -- "$path")
    [[ "$size" == "$payload" ]] || {
        echo "materialized synthetic payload has size $size, expected $payload" >&2
        return 1
    }
    digest "$path"
}

khala_mode() {
    [[ "$1" == nexus-rdma-go ]] && printf '%s\n' nexus-rdma || printf '%s\n' "$1"
}

attaches_shmem() {
    case "$1" in invm-py|invm-js|invm-go) return 1 ;; *) return 0 ;; esac
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
    git -C "$path" rev-parse --git-dir >/dev/null 2>&1 || { echo "missing $label repository at $path" >&2; exit 2; }
    local status
    status=$(git -C "$path" status --short)
    [[ -z "$status" ]] || { echo "$label repository is dirty; refusing claim-bearing run" >&2; printf '%s\n' "$status" >&2; exit 2; }
}

repo_value() {
    local field=$1 path=$2 override=${3:-}
    if git -C "$path" rev-parse --git-dir >/dev/null 2>&1; then
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
    [[ "$(git branch --show-current)" == "$expected_loader_branch" ]] || {
        echo "E2-Synth requires Loader branch $expected_loader_branch" >&2; exit 2; }
    [[ "$(git -C ../khala branch --show-current)" == "$expected_khala_branch" ]] || {
        echo "E2-Synth requires Khala branch $expected_khala_branch" >&2; exit 2; }
    [[ "$(git rev-parse HEAD)" == "$(git rev-parse "origin/$expected_loader_branch")" ]] || {
        echo "Loader feature branch is not pushed at the local HEAD" >&2; exit 2; }
    [[ "$(git -C ../khala rev-parse HEAD)" == "$(git -C ../khala rev-parse "origin/$expected_khala_branch")" ]] || {
        echo "Khala feature branch is not pushed at the local HEAD" >&2; exit 2; }
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
    printf 'CELL experiment=e2-synth cluster_id=%s phase=%s repetition=%s mode=%s workload=%s payload_bytes=%s rps=%s replicas=%s perf=%s warmup_minutes=%s measurement_minutes=%s output=%s\n' "$cluster_id" "$@"
}

write_config() {
    local experiment=$1 duration=$2 perf=$3 fixed=$4 trace_path=$5 output_prefix=$6 destination=$7
    EXPERIMENT="$experiment" EXP_DUR="$duration" WARMUP="$warmup_minutes" PREFETCH=false \
        ENABLE_PERF="$perf" FIXED_REPLICAS="$fixed" TRACE_PATH="$trace_path" \
        OUTPUT_PREFIX="$output_prefix" \
        envsubst < cmd/config_e2_synth_trace_template.json > "$destination"
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

validation_endpoint() {
    local mode=$1 worker_config=$2
    if [[ "$mode" == nexus-rdma-py || "$mode" == nexus-rdma-go ]]; then
        printf 'http://%s:10090\n' "$(jq -r '.storage_nodes[0]' "$worker_config")"
    elif [[ "$minio_endpoint" == http://* || "$minio_endpoint" == https://* ]]; then
        printf '%s\n' "$minio_endpoint"
    else
        printf 'http://%s\n' "$minio_endpoint"
    fi
}

capture_shmem_inventory() {
    local worker_config=$1 output=$2 host
    printf 'path,size_bytes\n' > "$output"
    while IFS= read -r host; do
        ssh -o BatchMode=yes -o StrictHostKeyChecking=accept-new "$host" \
            "sudo find /dev/shm/khala -maxdepth 1 -type f -name 'shmem-*.img' -printf '%p,%s\\n' | LC_ALL=C sort" >> "$output"
    done < <(jq -r '.worker_nodes[]' "$worker_config")
}

validate_shmem_inventory() {
    local mode=$1 inventory=$2 summary=$3 expected_count expected_size observed_count observed_total bad_sizes
    if attaches_shmem "$mode"; then expected_count=$replicas; expected_size=$vm_shmem_bytes; else expected_count=0; expected_size=0; fi
    observed_count=$(( $(wc -l < "$inventory") - 1 ))
    observed_total=$(awk -F, 'NR>1 {sum+=$2} END {print sum+0}' "$inventory")
    bad_sizes=$(awk -F, -v n="$expected_size" 'NR>1 && $2 != n {bad++} END {print bad+0}' "$inventory")
    {
        echo shmem_status=$([[ "$observed_count" == "$expected_count" && "$bad_sizes" == 0 ]] && echo PASS || echo FAIL)
        echo shmem_attached=$([[ "$expected_count" == 0 ]] && echo false || echo true)
        echo "vm_shmem_bytes=$expected_size"
        echo "observed_file_count=$observed_count"
        echo "observed_total_bytes=$observed_total"
    } > "$summary"
    [[ "$observed_count" == "$expected_count" && "$bad_sizes" == 0 && "$observed_total" == $((expected_count * expected_size)) ]]
}

validate_cleanup_residue() {
    local worker_config=$1 host
    while IFS= read -r host; do
        ssh -o BatchMode=yes -o StrictHostKeyChecking=accept-new "$host" \
            "test -z \"\$(sudo find /dev/shm/khala -maxdepth 1 -type f -name 'shmem-*.img' -print -quit)\"; test -z \"\$(find \"\$HOME\" -maxdepth 1 -type f -name 'perf.*' -print -quit)\"; ! pgrep -f '[b]in/kn-integration' >/dev/null; ! tmux has-session -t kn-integration 2>/dev/null"
    done < <(jq -r '.worker_nodes[]' "$worker_config")
    [[ -z "$(kubectl get kservice.serving.knative.dev,deployments.apps,replicasets.apps,pods,jobs.batch -n default -o name 2>/dev/null)" ]]
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
                "$(khala_artifact_hash "$kernel")" "$(khala_artifact_hash "$vmm")" "$(khala_artifact_hash bin/kn-integration)" "$(khala_artifact_hash bin/nexus-backend)" "$(khala_artifact_hash bin/hardware-manager)" "$expected_workload" "$expected_khala_branch" \
                "$(khala_artifact_hash bin/vm-orchestrator)" "$(khala_artifact_hash bin/dynamic-core-pool)" "$(khala_artifact_hash bin/khala-command)" "$(khala_artifact_hash bin/kn-integration-tracer)" <<'SH' >> "$output"
set -euo pipefail
host=$1 vm_config=$2 rootfs=$3 kernel=$4 vmm=$5 expected_head=$6 expected_config=$7 expected_rootfs=$8 expected_kernel=$9 expected_vmm=${10} expected_binary=${11} expected_nexus_backend=${12} expected_hardware_manager=${13} expected_workload=${14} expected_branch=${15} expected_vm_orchestrator=${16} expected_core_pool=${17} expected_khala_command=${18} expected_tracer=${19}
cd ~/khala
head=$(git rev-parse HEAD); branch=$(git branch --show-current); status=$(git status --porcelain)
[[ "$head" == "$expected_head" && "$branch" == "$expected_branch" && -z "$status" ]]
workload=$(git ls-files workload | LC_ALL=C sort | while IFS= read -r path; do sha256sum "$path"; done | sha256sum | awk '{print $1}')
[[ "$workload" == "$expected_workload" ]]
for item in "$vm_config:$expected_config" "$rootfs:$expected_rootfs" "$kernel:$expected_kernel" "$vmm:$expected_vmm" "bin/kn-integration:$expected_binary" "bin/nexus-backend:$expected_nexus_backend" "bin/hardware-manager:$expected_hardware_manager" "bin/vm-orchestrator:$expected_vm_orchestrator" "bin/dynamic-core-pool:$expected_core_pool" "bin/khala-command:$expected_khala_command" "bin/kn-integration-tracer:$expected_tracer"; do
    path=${item%%:*}; expected=${item#*:}; actual=$(sha256sum "$path" | awk '{print $1}'); [[ "$actual" == "$expected" ]]; printf 'role=worker host=%s tree=khala path=%s sha256=%s\n' "$host" "$path" "$actual"
done
cpu_model=$(lscpu | awk -F: '$1 == "Model name" {sub(/^[[:space:]]+/, "", $2); print $2; exit}')
printf 'role=worker host=%s cpu_model=%q online_cpus=%s perf_version=%q perf_event_paranoid=%s\n' \
    "$host" "$cpu_model" "$(nproc)" "$(perf --version)" "$(cat /proc/sys/kernel/perf_event_paranoid)"
printf 'role=worker host=%s tree=khala head=%s workload_sha256=%s status=clean\n' "$host" "$head" "$workload"
SH
        done
    done
    for host in "${provenance_loaders[@]}"; do
        ssh -o BatchMode=yes -o StrictHostKeyChecking=accept-new -o ConnectTimeout=5 "$host" bash -s -- "$host" "$expected_invitro_head" "$expected_loader_branch" "$repo_root" <<'SH' >> "$output"
set -euo pipefail
host=$1 expected_head=$2 expected_branch=$3 repo=$4; cd "$repo"; head=$(git rev-parse HEAD); branch=$(git branch --show-current)
test "$head" = "$expected_head"; test "$branch" = "$expected_branch"; test -z "$(git status --porcelain)"
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
    local directory=$1 listed actual
    lifecycle_archived_output_matches "$directory" || return 1
    listed=$(mktemp); actual=$(mktemp)
    tail -n +2 "$directory/archived-output-checksums.csv" | cut -d, -f1 | LC_ALL=C sort > "$listed"
    find "$directory" -type f ! -name manifest.txt ! -name archived-output-checksums.csv \
        -printf '%P\n' | LC_ALL=C sort > "$actual"
    cmp --silent "$listed" "$actual"
    local status=$?
    rm -f -- "$listed" "$actual"
    return "$status"
}

manifest_matches() {
    local manifest=$1 phase=$2 repetition=$3 mode=$4 workload=$5 rps=$6 perf=$7 duration=$8 destination=$9
    local vm_config rootfs kernel vmm worker_count expected_perf_artifacts admission_workload payload attached expected_vm expected_ring seeded_path
    [[ -f "$manifest" ]] || return 1
    vm_config=$(mode_vm_config "$mode")
    rootfs=$(config_value "../khala/$vm_config" RootfsPath)
    kernel=$(config_value "../khala/$vm_config" KernelPath)
    vmm=$(config_value "../khala/$vm_config" FirecrackerPath)
    admission_workload=$(trace_workload_identity "$destination/trace/invocations.csv") || return 1
    payload=${workload##*_p_}
    seeded_path=$(seeded_payload_path "$payload")
    [[ -f "$seeded_path" && ! -L "$seeded_path" ]] || return 1
    [[ "$(stat -c '%s' -- "$seeded_path")" == "$payload" ]] || return 1
    attached=false; expected_vm=0; expected_ring=0
    if attaches_shmem "$mode"; then attached=true; expected_vm=$vm_shmem_bytes; expected_ring=$shmem_ring_bytes; fi
        line_is "$manifest" 'manifest_version=2' &&
        line_is "$manifest" 'experiment=e2-synth' && line_is "$manifest" "cluster_id=$cluster_id" &&
        line_is "$manifest" "smoke=$smoke" &&
        line_is "$manifest" "phase=$phase" && line_is "$manifest" "repetition=$repetition" &&
        line_is "$manifest" "mode=$mode" && line_is "$manifest" "workload=$workload" && line_is "$manifest" "payload_bytes=$payload" &&
        line_is "$manifest" "rps=$rps" && line_is "$manifest" "offered_rps=$rps" && line_is "$manifest" "replicas=$replicas" &&
        line_is "$manifest" "shmem_attached=$attached" && line_is "$manifest" "vm_shmem_bytes=$expected_vm" && line_is "$manifest" "shmem_ring_bytes=$expected_ring" &&
        line_is "$manifest" "perf_enabled=$perf" && line_is "$manifest" "profile=$profile" &&
        line_is "$manifest" 'external_lifecycle_cleanup=true' &&
        line_is "$manifest" "admission_workload=$admission_workload" &&
        line_is "$manifest" "minio_endpoint=$minio_endpoint" &&
        line_is "$manifest" "warmup_minutes=$warmup_minutes" && line_is "$manifest" "measurement_minutes=$duration" &&
        line_is "$manifest" 'scan_snapshot=false' &&
        line_is "$manifest" "ceiling_multiplier=$ceiling_multiplier" &&
        line_is "$manifest" "invitro_head=$(git rev-parse HEAD)" &&
        line_is "$manifest" "khala_head=$(git -C ../khala rev-parse HEAD)" &&
        line_is "$manifest" "firecracker_head=$(repo_value head ../firecracker "$eval_firecracker_head")" &&
        line_is "$manifest" "rdma_demo_head=$(repo_value head ../rdma-demo "$eval_rdma_demo_head")" &&
        line_is "$manifest" "e1_summary_sha256=$(digest "$e1_summary")" &&
        line_is "$manifest" "calibrator_sha256=$(digest e2_synth_calibrate_rps.py)" &&
        line_is "$manifest" "runner_sha256=$(digest run_e2_synth.sh)" &&
        line_is "$manifest" "evidence_validator_sha256=$(digest experiment/e2synth/validate_evidence.py)" &&
        line_is "$manifest" "config_template_sha256=$(digest cmd/config_e2_synth_trace_template.json)" &&
        line_is "$manifest" "seeded_input_sha256=$(digest "$seeded_path")" &&
        line_is "$manifest" "output_validator_sha256=$(digest ../khala/scripts/validate_e2_synth_output.sh)" &&
        line_is "$manifest" "vm_config_path=$vm_config" &&
        line_is "$manifest" "vm_config_sha256=$(khala_artifact_hash "$vm_config")" &&
        line_is "$manifest" "rootfs_path=$rootfs" && line_is "$manifest" "rootfs_sha256=$(khala_artifact_hash "$rootfs")" &&
        line_is "$manifest" "kernel_path=$kernel" && line_is "$manifest" "kernel_sha256=$(khala_artifact_hash "$kernel")" &&
        line_is "$manifest" "vmm_path=$vmm" && line_is "$manifest" "vmm_sha256=$(khala_artifact_hash "$vmm")" &&
        line_is "$manifest" "workload_sha256=$(tracked_workload_sha)" &&
        line_is "$manifest" "remote_provenance_sha256=$(digest "$result_root/remote-provenance.txt")" &&
        line_is "$manifest" "cluster_inventory_sha256=$(digest "$result_root/cluster-inventory.txt")" &&
        line_is "$manifest" "worker_config_sha256=$(digest "$result_root/worker-node.json")" &&
        line_is "$manifest" "worker_map_sha256=$(digest "$result_root/worker-node.json")" &&
        line_is "$manifest" 'evidence_status=0' && line_is "$manifest" 'output_validation_status=0' && line_is "$manifest" 'shmem_validation_status=0' &&
        line_is "$manifest" 'admission_status=0' &&
        line_is "$manifest" "admission_expected_replicas=$replicas" &&
        line_is "$manifest" 'snapshot_status=0' &&
        line_is "$manifest" 'snapshot_workload_count=1' &&
        line_is "$manifest" 'acquisition_started=true' &&
        line_is "$manifest" 'exit_status=0' || return 1
    lifecycle_success_manifest_matches "$manifest" || return 1
    snapshot_cleanup_policy_matches "$manifest" || return 1
    worker_count=$(jq '.worker_nodes | length' "$destination/worker-node.json")
    expected_perf_artifacts=0
    if [[ "$perf" == true ]]; then expected_perf_artifacts=$((worker_count * 4)); fi
    line_is "$manifest" "perf_artifact_count=$expected_perf_artifacts" || return 1
    line_is "$manifest" "evidence_validation_sha256=$(digest "$destination/evidence-validation.txt")" || return 1
    line_is "$manifest" "output_validation_sha256=$(digest "$destination/synthetic-byte-validation.csv")" || return 1
    line_is "$manifest" "shmem_inventory_sha256=$(digest "$destination/shmem-files.csv")" || return 1
    line_is "$manifest" "shmem_validation_sha256=$(digest "$destination/shmem-validation.txt")" || return 1
    line_is "$manifest" "admission_evidence_sha256=$(digest "$destination/admission-validation.txt")" || return 1
    line_is "$manifest" "admission_readiness_sha256=$(digest "$destination/admission.csv")" || return 1
    line_is "$manifest" "archive_checksums_sha256=$(digest "$destination/archived-output-checksums.csv")" || return 1
    if [[ "$phase" == collection ]]; then
        line_is "$manifest" "reference_sha256=$(digest "$reference")" || return 1
        cmp --silent "$reference" "$result_root/e2-synth-rps-reference.csv" || return 1
    fi
    cmp --silent "$destination/remote-provenance.txt" "$result_root/remote-provenance.txt" || return 1
    cmp --silent "$destination/cluster-inventory.txt" "$result_root/cluster-inventory.txt" || return 1
    cmp --silent "$destination/worker-node.json" "$result_root/worker-node.json" || return 1
    cmp --silent "$destination/e1-synth-unloaded-average.csv" "$e1_summary" || return 1
    if [[ "$phase" == collection ]]; then
        cmp --silent "$destination/e2-synth-rps-reference.csv" "$reference" || return 1
    fi
    archived_output_matches "$destination"
}

prepare_cluster_root() {
    local require_reference=$1 check_dir
    if [[ -e "$result_root" ]]; then
        [[ -f "$result_root/cluster-inventory.txt" && -f "$result_root/worker-node.json" && -f "$result_root/remote-provenance.txt" && -f "$result_root/e1-synth-unloaded-average.csv" ]] || {
            echo "existing E2-Synth result root lacks archived cluster provenance" >&2; return 2; }
        cmp --silent "$e1_summary" "$result_root/e1-synth-unloaded-average.csv" || {
            echo "archived E1 reference differs from this run" >&2; return 2; }
        if [[ "$require_reference" == true ]]; then
            [[ -f "$result_root/e2-synth-rps-reference.csv" ]] && cmp --silent "$reference" "$result_root/e2-synth-rps-reference.csv" || {
                echo "archived E2-Synth reference differs from collection input" >&2; return 2; }
        fi
        check_dir=$(mktemp -d)
        discover_cluster_topology "$check_dir/cluster-inventory.txt" "$check_dir/worker-node.json"
        cmp --silent "$check_dir/cluster-inventory.txt" "$result_root/cluster-inventory.txt" &&
            cmp --silent "$check_dir/worker-node.json" "$result_root/worker-node.json" || {
                rm -r -- "$check_dir"; echo "live E2-Synth topology differs from archived result root" >&2; return 2; }
        snapshot_remote_provenance "$check_dir/remote-provenance.txt" "$check_dir/worker-node.json" "$require_reference"
        cmp --silent "$check_dir/remote-provenance.txt" "$result_root/remote-provenance.txt" || {
            rm -r -- "$check_dir"; echo "remote provenance differs from archived result root" >&2; return 2; }
        rm -r -- "$check_dir"
    else
        mkdir -p "$result_root"
        cp -- "$e1_summary" "$result_root/e1-synth-unloaded-average.csv"
        if [[ "$require_reference" == true ]]; then cp -- "$reference" "$result_root/e2-synth-rps-reference.csv"; fi
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
        line_is "$manifest" "runner_sha256=$(digest run_e2_synth.sh)" &&
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
        echo "runner_sha256=$(digest run_e2_synth.sh)"
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
    local payload=${workload##*_p_}
    local run_id="e2-synth-${phase}-r${repetition}-${mode}-p${payload}"
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
    if [[ -f "$scratch_out/acquisition-started.marker" ]]; then
        mkdir -p "$(dirname "$destination")"
        cp -a -- "$scratch_cell" "$destination"
        {
            echo manifest_version=2
            echo experiment=e2-synth
            echo "cluster_id=$cluster_id"
            echo "phase=$phase"
            echo "repetition=$repetition"
            echo "mode=$mode"
            echo "workload=$workload"
            echo "payload_bytes=$payload"
            echo stale_post_acquisition_scratch=true
            echo acquisition_started=true
            echo exit_status=71
        } > "$destination/manifest.txt"
        write_archived_output_checksums "$destination"
        echo "sealed stale post-acquisition scratch; refusing replay: $destination" >&2
        return 71
    fi
    local evidence_status=1 output_validation_status=1 shmem_validation_status=1 perf_artifact_count=0 admission_status=1 admission_function_count=0 admission_workload=
    local seeded_input_path=$(seeded_payload_path "$payload") seeded_input_sha256=
    local admission_aggregate_expected=0 admission_aggregate_ready=0 snapshot_status=1
    local stale_scratch=false snapshot_cleanup_policy=
    local acquisition_marker="$scratch_out/acquisition-started.marker"
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
            local previous_attempt=$((attempt - 1))
            local preserved="$scratch_out/setup-attempt-$previous_attempt"
            mkdir -p "$preserved"
            find "$scratch_out" -mindepth 1 -maxdepth 1 ! -name 'setup-attempt-*' -exec cp -a -- {} "$preserved" \;
            rm -rf -- "$scratch_trace" "$scratch_out/trace"
            rm -f -- "$scratch_out/trace-generator.log" "$config_path" "$scratch_out/manifest.txt"
            rm -f -- "$acquisition_marker" "$scratch_out/admission.csv" "$scratch_out/admission-validation.txt" \
                "$scratch_out/admission-deployments.json" "$scratch_out/admission-poll.txt"
        fi
        seeded_input_sha256=$(materialize_selected_payload "$payload") || return 1
        [[ -n "$seeded_input_sha256" ]] || return 1
        if [[ "$phase" == calibration ]]; then
            python3 e2_synth_calibrate_rps.py --averages "$e1_summary" --cores "$worker_cores" --ceiling-multiplier "$ceiling_multiplier" trace \
                --payload "$payload" --warmup-minutes "$warmup_minutes" --steps "$steps" --minutes-per-step "$minutes_per_step" --output "$scratch_trace"
        else
            python3 e2_synth_calibrate_rps.py --averages "$e1_summary" --cores "$worker_cores" --ceiling-multiplier "$ceiling_multiplier" fixed-trace \
                --payload "$payload" --mode "$mode" --rps "$rps" --warmup-minutes "$warmup_minutes" \
                --measurement-minutes "$duration" --output "$scratch_trace"
        fi
        admission_workload=$(trace_workload_identity "$scratch_trace/invocations.csv") || {
            echo "fixed trace contains no unique admission workload identity" >&2
            return 2
        }
        [[ "$admission_workload" == "$(trace_workload "$payload" "$mode")" ]] || {
            echo "trace identity does not match canonical mode/payload" >&2
            return 2
        }
        write_config "$run_id" "$duration" "$perf" "$replicas" "$scratch_trace" "$scratch_out/experiment" "$config_path"
        cp -a -- "$scratch_trace" "$scratch_out/trace"
        cp -- "$result_root/remote-provenance.txt" "$scratch_out/remote-provenance.txt"
        cp -- "$result_root/cluster-inventory.txt" "$scratch_out/cluster-inventory.txt"
        cp -- "$worker_config" "$scratch_out/worker-node.json"
        cp -- "$e1_summary" "$scratch_out/e1-synth-unloaded-average.csv"
        if [[ "$phase" == collection ]]; then cp -- "$reference" "$scratch_out/e2-synth-rps-reference.csv"; fi
        {
        vm_config=$(mode_vm_config "$mode")
        rootfs=$(config_value "../khala/$vm_config" RootfsPath)
        kernel=$(config_value "../khala/$vm_config" KernelPath)
        vmm=$(config_value "../khala/$vm_config" FirecrackerPath)
        echo manifest_version=2
        echo experiment=e2-synth
        echo "cluster_id=$cluster_id"
        echo "smoke=$smoke"
        echo "phase=$phase"
        echo "repetition=$repetition"
        echo "mode=$mode"
        echo "workload=$workload"
        echo "payload_bytes=$payload"
        echo "rps=$rps"
        echo "offered_rps=$rps"
        echo "replicas=$replicas"
        echo "perf_enabled=$perf"
        echo 'external_lifecycle_cleanup=true'
        echo "admission_workload=$admission_workload"
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
        if attaches_shmem "$mode"; then
            echo shmem_attached=true
            echo "vm_shmem_bytes=$vm_shmem_bytes"
            echo "shmem_ring_bytes=$shmem_ring_bytes"
        else
            echo shmem_attached=false
            echo vm_shmem_bytes=0
            echo shmem_ring_bytes=0
        fi
        echo "shmem_io_quantum=$shmem_io_quantum"
        echo "e1_summary_sha256=$(digest "$e1_summary")"
        echo "calibrator_sha256=$(digest e2_synth_calibrate_rps.py)"
        echo "runner_sha256=$(digest run_e2_synth.sh)"
        echo "evidence_validator_sha256=$(digest experiment/e2synth/validate_evidence.py)"
        echo "config_template_sha256=$(digest cmd/config_e2_synth_trace_template.json)"
        echo "trace_modes_sha256=$(digest e2_synth_modes.py)"
        echo "seeded_input_sha256=$seeded_input_sha256"
        echo "output_validator_sha256=$(digest ../khala/scripts/validate_e2_synth_output.sh)"
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
        local attempt=$1 deploy_mode vm_bytes endpoint
        deploy_mode=$(khala_mode "$mode")
        vm_bytes=0; attaches_shmem "$mode" && vm_bytes=$vm_shmem_bytes
        SIZES="$payload" go run experiment/khala_command.go --command deploy --mode "$deploy_mode" --worker-config "$worker_config" --workloads "$workload" \
            --vm-shmem-bytes "$vm_bytes" --shmem-ring-bytes "$shmem_ring_bytes" --shmem-io-quantum "$shmem_io_quantum" --minio-endpoint "$minio_endpoint" \
            2>&1 | tee "$scratch_out/deploy-attempt-$attempt.log"
        local status=${PIPESTATUS[0]}
        cat "$scratch_out/deploy-attempt-$attempt.log" >> "$scratch_out/deploy.log"
        if ((status == 0)); then
            [[ "$(digest "$seeded_input_path")" == "$seeded_input_sha256" ]] || {
                echo "seeded synthetic payload changed after deploy" >&2
                return 1
            }
            go run experiment/khala_command.go --command create-snapshots --mode "$deploy_mode" \
                --worker-config "$worker_config" --workloads "$workload" \
                --vm-shmem-bytes "$vm_bytes" --shmem-ring-bytes "$shmem_ring_bytes" --shmem-io-quantum "$shmem_io_quantum" \
                2>&1 | tee "$scratch_out/snapshot-attempt-$attempt.log"
            local snapshot_result=${PIPESTATUS[0]}
            cat "$scratch_out/snapshot-attempt-$attempt.log" >> "$scratch_out/snapshot.log"
            snapshot_status=$snapshot_result
            if ((snapshot_result != 0)); then status=$snapshot_result; fi
            if ((status == 0)); then
                endpoint=$(validation_endpoint "$mode" "$worker_config")
                ../khala/scripts/validate_e2_synth_output.sh clear --endpoint "$endpoint" --mode "$mode" --payload "$payload" \
                    > "$scratch_out/output-clear-attempt-$attempt.log" 2>&1
                status=$?
            fi
        fi
        return "$status"
    }
    lifecycle_run() {
        rm -f -- "$acquisition_marker"
        go run cmd/loader.go --config "$config_path" \
            --external-lifecycle-cleanup \
            --e2-admission-workload "$admission_workload" --e2-admission-replicas "$replicas" \
            --e2-admission-output "$scratch_out/admission" --e2-acquisition-marker "$acquisition_marker" \
            > >(tee "$scratch_out/loader.log") 2>&1
        local status=$?
        if [[ -f "$acquisition_marker" ]]; then LIFECYCLE_ACQUISITION_STARTED=true; else LIFECYCLE_ACQUISITION_STARTED=false; fi
        admission_status=1
        if grep -Fqx 'admission_status=PASS' "$scratch_out/admission-validation.txt" 2>/dev/null; then admission_status=0; fi
        admission_function_count=$(awk -F= '$1 == "admission_function_count" {print $2}' "$scratch_out/admission-validation.txt" 2>/dev/null || true)
        admission_aggregate_expected=$(awk -F= '$1 == "admission_aggregate_expected_replicas" {print $2}' "$scratch_out/admission-validation.txt" 2>/dev/null || true)
        admission_aggregate_ready=$(awk -F= '$1 == "admission_aggregate_ready_replicas" {print $2}' "$scratch_out/admission-validation.txt" 2>/dev/null || true)
        admission_function_count=${admission_function_count:-0}
        admission_aggregate_expected=${admission_aggregate_expected:-0}
        admission_aggregate_ready=${admission_aggregate_ready:-0}
        local endpoint inventory_status
        endpoint=$(validation_endpoint "$mode" "$worker_config")
        ../khala/scripts/validate_e2_synth_output.sh validate --endpoint "$endpoint" --mode "$mode" --payload "$payload" \
            --output "$scratch_out/synthetic-byte-validation.csv" > "$scratch_out/output-validation.log" 2>&1
        output_validation_status=$?
        capture_shmem_inventory "$worker_config" "$scratch_out/shmem-files.csv"
        inventory_status=$?
        if ((inventory_status == 0)); then
            validate_shmem_inventory "$mode" "$scratch_out/shmem-files.csv" "$scratch_out/shmem-validation.txt"
            shmem_validation_status=$?
        else
            printf 'shmem_status=FAIL\nshmem_attached=%s\nvm_shmem_bytes=%s\nobserved_file_count=0\nobserved_total_bytes=0\n' \
                "$([[ "$mode" == invm-* ]] && echo false || echo true)" "$([[ "$mode" == invm-* ]] && echo 0 || echo "$vm_shmem_bytes")" > "$scratch_out/shmem-validation.txt"
            shmem_validation_status=$inventory_status
        fi
        python3 experiment/e2synth/validate_evidence.py --output-prefix "$scratch_out/experiment" \
            --loader-log "$scratch_out/loader.log" --worker-config "$worker_config" --perf-enabled "$perf" \
            > "$scratch_out/evidence-validation.txt" 2>&1
        evidence_status=$?
        perf_artifact_count=$(awk -F= '$1 == "perf_artifact_count" {print $2}' "$scratch_out/evidence-validation.txt")
        perf_artifact_count=${perf_artifact_count:-0}
        if ((status == 0 && evidence_status != 0)); then status=$evidence_status; fi
        if ((status == 0 && output_validation_status != 0)); then status=$output_validation_status; fi
        if ((status == 0 && shmem_validation_status != 0)); then status=$shmem_validation_status; fi
        return "$status"
    }
    lifecycle_cleanup() {
        local cleanup_phase=$1 policy remove_snapshots=false deploy_mode endpoint helper_status=0 clean_status=0 residue_status=0
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
        deploy_mode=$(khala_mode "$mode")
        if [[ "$cleanup_phase" == final ]]; then
            endpoint=$(validation_endpoint "$mode" "$worker_config")
            ../khala/scripts/validate_e2_synth_output.sh clear --endpoint "$endpoint" --mode "$mode" --payload "$payload" \
                > "$scratch_out/output-clear-$cleanup_phase.log" 2>&1
            helper_status=$?
        fi
        go run experiment/khala_command.go --command clean --mode "$deploy_mode" --worker-config "$worker_config" --minio-endpoint "$minio_endpoint" \
            --remove-snapshots="$remove_snapshots" > "$scratch_out/clean-$cleanup_phase.log" 2>&1
        clean_status=$?
        cat "$scratch_out/clean-$cleanup_phase.log" >> "$scratch_out/clean.log"
        validate_cleanup_residue "$worker_config" >> "$scratch_out/clean-$cleanup_phase.log" 2>&1
        residue_status=$?
        ((helper_status == 0)) || return "$helper_status"
        ((clean_status == 0)) || return "$clean_status"
        return "$residue_status"
    }
    lifecycle_finalize() {
        local status=$1 clean_status=$2 setup_attempts=$3 deploy_invocations=$4 loader_started=$5
        local manifest_status=$status
        if ((clean_status != 0)); then manifest_status=$LIFECYCLE_CLEANUP_ABORT; fi
        if [[ ! -f "$scratch_out/evidence-validation.txt" ]]; then
            printf '%s\n' 'evidence_status=FAIL reason=loader did not run' > "$scratch_out/evidence-validation.txt"
        fi
        {
            echo "evidence_status=$evidence_status"
            echo "output_validation_status=$output_validation_status"
            echo "shmem_validation_status=$shmem_validation_status"
            echo "perf_artifact_count=$perf_artifact_count"
            echo "evidence_validation_sha256=$(digest "$scratch_out/evidence-validation.txt")"
            if [[ -f "$scratch_out/synthetic-byte-validation.csv" ]]; then
                echo "output_validation_sha256=$(digest "$scratch_out/synthetic-byte-validation.csv")"
            else
                echo output_validation_sha256=
            fi
            if [[ -f "$scratch_out/shmem-files.csv" ]]; then
                echo "shmem_inventory_sha256=$(digest "$scratch_out/shmem-files.csv")"
            else
                echo shmem_inventory_sha256=
            fi
            if [[ -f "$scratch_out/shmem-validation.txt" ]]; then
                echo "shmem_validation_sha256=$(digest "$scratch_out/shmem-validation.txt")"
            else
                echo shmem_validation_sha256=
            fi
            if [[ -f "$scratch_out/admission-validation.txt" ]]; then
                echo "admission_evidence_sha256=$(digest "$scratch_out/admission-validation.txt")"
            else
                echo 'admission_evidence_sha256='
            fi
            if [[ -f "$scratch_out/admission.csv" ]]; then
                echo "admission_readiness_sha256=$(digest "$scratch_out/admission.csv")"
            else
                echo 'admission_readiness_sha256='
            fi
            echo "admission_status=$admission_status"
            echo "admission_expected_replicas=$replicas"
            echo "admission_function_count=$admission_function_count"
            echo "admission_aggregate_expected_replicas=$admission_aggregate_expected"
            echo "admission_aggregate_ready_replicas=$admission_aggregate_ready"
            echo "snapshot_status=$snapshot_status"
            echo 'snapshot_workload_count=1'
            echo "snapshot_workloads=$workload"
            echo "setup_attempts=$setup_attempts"
            echo "deploy_attempts=$deploy_invocations"
            echo "deploy_invocations=$deploy_invocations"
            echo "loader_started=$loader_started"
            echo "acquisition_started=$LIFECYCLE_ACQUISITION_STARTED"
            echo "cleanup_exit_status=$clean_status"
            echo "snapshot_cleanup_policy=$snapshot_cleanup_policy"
            echo 'acquisition_retry=false'
            echo 'independent_continuation=true'
            if [[ "$loader_started" == true ]]; then echo 'lifecycle_phase=final'; else echo 'lifecycle_phase=setup'; fi
        echo "end_utc=$(date -u --iso-8601=seconds)"
        echo "exit_status=$manifest_status"
        } >> "$scratch_out/manifest.txt"
        mkdir -p "$(dirname "$destination")"
        cp -a -- "$scratch_out" "$destination"
        write_archived_output_checksums "$destination"
        printf 'archive_checksums_sha256=%s\n' "$(digest "$destination/archived-output-checksums.csv")" >> "$destination/manifest.txt"
        printf 'archived_output_checksums_sha256=%s\n' "$(digest "$destination/archived-output-checksums.csv")" >> "$destination/manifest.txt"
        printf '%s\n' 'worker_map_path=worker-node.json' 'archive_checksums_path=archived-output-checksums.csv' >> "$destination/manifest.txt"
        echo "worker_map_sha256=$(digest "$worker_config")" >> "$destination/manifest.txt"
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
    [[ "${modes[*]}" == invm-py ]] || { echo "E2-Synth calibration permits only invm-py" >&2; exit 2; }
    calibration_minutes=$((steps * minutes_per_step))
    if [[ "$dry_run" == true ]]; then
        for ((repetition=0; repetition<repetitions; repetition++)); do
            repetition_root=$result_root; ((repetitions == 1)) || repetition_root="$result_root/rep-$repetition"
            for payload in "${payloads[@]}"; do
                workload=$(base_workload "$payload")
                print_cell calibration "$repetition" invm-py "$workload" "$payload" sweep "$replicas" false \
                    "$warmup_minutes" "$calibration_minutes" "$repetition_root/cells/p-$payload"
            done
        done
        echo "E2_SYNTH_CALIBRATION_DRY_RUN_READY cells=$((${#payloads[@]} * repetitions))"
        exit 0
    fi
    validate_claim_sources
    prepare_cluster_root false
    scripts/util/wait_prometheus_ready.sh
    run_initial_cleanup || { status=$?; echo "initial cleanup failed; refusing E2-Synth acquisition" >&2; exit "$status"; }
    suite_failed=false
    for ((repetition=0; repetition<repetitions; repetition++)); do
        repetition_root=$result_root; ((repetitions == 1)) || repetition_root="$result_root/rep-$repetition"
        mkdir -p "$repetition_root"
        python3 e2_synth_calibrate_rps.py --averages "$e1_summary" --cores "$worker_cores" --ceiling-multiplier "$ceiling_multiplier" plan \
            --payloads "$payloads_text" --steps "$steps" --minutes-per-step "$minutes_per_step" --output "$repetition_root/calibration-plan.csv"
        observations=(); repetition_failed=false
        for payload in "${payloads[@]}"; do
            workload=$(base_workload "$payload"); cell="$repetition_root/cells/p-$payload"
            if run_cell calibration "$repetition" invm-py "$workload" sweep false "$calibration_minutes" "$cell" "$result_root/worker-node.json"; then
                :
            else
                status=$?; [[ "$LIFECYCLE_CLEANUP_FAILED" != true ]] || exit "$status"
                suite_failed=true; repetition_failed=true; continue
            fi
            duration_csv=$(find "$cell" -maxdepth 1 -name 'experiment_duration_*.csv' -print -quit)
            [[ -n "$duration_csv" ]] || { echo "missing duration CSV for payload $payload" >&2; suite_failed=true; repetition_failed=true; continue; }
            observation="$cell/observations.csv"
            python3 e2_synth_calibrate_rps.py --averages "$e1_summary" --cores "$worker_cores" --ceiling-multiplier "$ceiling_multiplier" observe \
                --payload "$payload" --duration-csv "$duration_csv" --failure-threshold "$failure_threshold" --slo-multiplier "$slo_multiplier" \
                --steps "$steps" --minutes-per-step "$minutes_per_step" --output "$observation"
            observations+=("$observation")
        done
        [[ "$repetition_failed" != true ]] || continue
        python3 e2_synth_calibrate_rps.py --averages "$e1_summary" --cores "$worker_cores" --ceiling-multiplier "$ceiling_multiplier" finalize \
            --cluster-id "$cluster_id" --payloads "$payloads_text" --observations "${observations[@]}" \
            --failure-threshold "$failure_threshold" --slo-multiplier "$slo_multiplier" \
            --output "$repetition_root/e2-synth-rps-reference-partial.csv"
        echo "E2_SYNTH_CALIBRATION_READY repetition=$repetition reference=$repetition_root/e2-synth-rps-reference-partial.csv"
    done
    [[ "$suite_failed" != true ]] || { echo "E2_SYNTH_CALIBRATION_FAILED result_root=$result_root" >&2; exit 1; }
    exit 0
fi

[[ -f "$reference" ]] || { echo "collect requires --reference" >&2; exit 2; }
[[ -f "$e1_summary" ]] || { echo "collect requires --e1-summary" >&2; exit 2; }
if [[ "$smoke" == true ]]; then
    [[ "$replicas" == 2 && "$measurement_minutes" == 1 && "$warmup_minutes" == 2 && "$repetitions" == 1 ]] || {
        echo "E2-Synth smoke requires two replicas, two warmup minutes, one measurement minute, and one pass" >&2; exit 2; }
fi
if [[ -z "$worker_cores" ]]; then worker_cores=${WORKER_CORES:-$(reference_unique_value worker_cores)}; fi
ceiling_multiplier=$(reference_unique_value ceiling_multiplier)
for payload in "${payloads[@]}"; do reference_value "$payload" rref >/dev/null; done
if [[ "$dry_run" != true ]]; then
    validate_claim_sources
    prepare_cluster_root true
    scripts/util/wait_prometheus_ready.sh
    run_initial_cleanup || { status=$?; echo "initial cleanup failed; refusing E2-Synth acquisition" >&2; exit "$status"; }
fi

suite_failed=false
for ((repetition=0; repetition<repetitions; repetition++)); do
    for ((payload_index=0; payload_index<${#payloads[@]}; payload_index++)); do
        payload=${payloads[payload_index]}; workload=$(base_workload "$payload"); rps=$(reference_value "$payload" rref)
        ordered_modes=("${modes[@]}")
        if ((payload_index % 2 == 1)); then
            for ((left=0, right=${#ordered_modes[@]}-1; left<right; left++, right--)); do
                swap=${ordered_modes[left]}; ordered_modes[left]=${ordered_modes[right]}; ordered_modes[right]=$swap
            done
        fi
        for mode in "${ordered_modes[@]}"; do
            destination="$result_root/rep-$repetition/p-$payload/$mode"
            if [[ "$dry_run" == true ]]; then
                print_cell collection "$repetition" "$mode" "$workload" "$payload" "$rps" "$replicas" true "$warmup_minutes" "$measurement_minutes" "$destination"
            elif run_cell collection "$repetition" "$mode" "$workload" "$rps" true "$measurement_minutes" "$destination" "$result_root/worker-node.json"; then
                :
            else
                status=$?; [[ "$LIFECYCLE_CLEANUP_FAILED" != true ]] || exit "$status"; suite_failed=true
            fi
        done
    done
done
if [[ "$dry_run" == true ]]; then
    echo "E2_SYNTH_DRY_RUN_READY cells=$((${#modes[@]} * ${#payloads[@]} * repetitions)) result_root=$result_root"
elif [[ "$suite_failed" == true ]]; then
    echo "E2_SYNTH_COLLECTION_FAILED result_root=$result_root" >&2; exit 1
else
    echo "E2_SYNTH_COLLECTION_READY result_root=$result_root"
fi
