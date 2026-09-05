#!/usr/bin/env bash

# An archived acquisition marker is the authoritative boundary after which an
# E2-Synth cell and the surrounding suite may not advance automatically.
e2_synth_acquisition_started() {
    local archived_cell=$1
    [[ -f "$archived_cell/acquisition-started.marker" ]] ||
        [[ -f "$archived_cell/out/acquisition-started.marker" ]] ||
        grep -Fqx 'acquisition_started=true' "$archived_cell/manifest.txt" 2>/dev/null
}

e2_synth_scratch_namespace() {
    local cluster_id=$1 canonical_result_root=$2
    [[ -n "$cluster_id" && "$canonical_result_root" == /* ]] || return 2
    printf '%s\0%s' "$cluster_id" "$canonical_result_root" | sha256sum | awk '{print $1}'
}

e2_synth_zero_payload_digest() {
    local payload_bytes=$1
    [[ "$payload_bytes" =~ ^[1-9][0-9]*$ ]] || return 2
    head -c "$payload_bytes" /dev/zero | sha256sum | awk '{print $1}'
}

e2_synth_validate_zero_payload() {
    local path=$1 payload_bytes=$2 size actual expected
    [[ -f "$path" && ! -L "$path" ]] || return 1
    size=$(stat -c '%s' -- "$path")
    [[ "$size" == "$payload_bytes" ]] || return 1
    actual=$(sha256sum "$path" | awk '{print $1}')
    expected=$(e2_synth_zero_payload_digest "$payload_bytes") || return 1
    [[ "$actual" == "$expected" ]] || return 1
    printf '%s\n' "$actual"
}

# Stage E2-Synth inputs before the shared deployment command starts the RDMA
# server. Keeping this in the standalone runner preserves legacy E2 behavior.
e2_synth_stage_rdma_payloads() {
    local khala_root=$1 worker_config=$2 storage source destination
    local -a mappings=(
        "assets/nexus-benchmark-payload/input_payload|rdma-demo/assets/nexus-benchmark-payload/input_payload"
        "assets/nexus-benchmark-payload/test|rdma-demo/assets/nexus-benchmark-payload/test"
        "assets/synthetic-payload|rdma-demo/assets/synthetic-payload-input"
    )
    local -a storage_nodes=()
    [[ -d "$khala_root" && -f "$worker_config" ]] || return 2
    ((${#mappings[@]} == 3)) || return 2
    mapfile -t storage_nodes < <(jq -er '.storage_nodes[]' "$worker_config")
    ((${#storage_nodes[@]} > 0)) || return 2
    for storage in "${storage_nodes[@]}"; do
        [[ -n "$storage" && "$storage" != null ]] || return 2
        ssh -o BatchMode=yes -o StrictHostKeyChecking=accept-new -o ConnectTimeout=5 "$storage" \
            'rm -rf -- "$HOME/rdma-demo/assets/nexus-benchmark-payload/output_payload" "$HOME/rdma-demo/assets/synthetic-payload-output"; mkdir -p -- "$HOME/rdma-demo/assets/nexus-benchmark-payload/input_payload" "$HOME/rdma-demo/assets/nexus-benchmark-payload/test" "$HOME/rdma-demo/assets/synthetic-payload-input"' || return
        for mapping in "${mappings[@]}"; do
            source=${mapping%%|*}
            destination=${mapping#*|}
            [[ -d "$khala_root/$source" ]] || return 2
            rsync -a --delete -e 'ssh -o BatchMode=yes -o StrictHostKeyChecking=accept-new -o ConnectTimeout=5' -- \
                "$khala_root/$source/" "$storage:$destination/" || return
        done
    done
}
