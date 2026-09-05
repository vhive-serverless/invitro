#!/usr/bin/env bash

# An archived acquisition marker is the authoritative boundary after which an
# E2-Synth cell and the surrounding suite may not advance automatically.
e2_synth_acquisition_started() {
    local archived_cell=$1
    [[ -f "$archived_cell/acquisition-started.marker" ]]
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
