#!/usr/bin/env bash
# Shared lifecycle for E2/E3 cells.  Callers provide the callbacks below:
# lifecycle_setup attempt; lifecycle_deploy attempt; lifecycle_run;
# lifecycle_cleanup phase; lifecycle_finalize run_status cleanup_status setup_attempts
# deploy_invocations loader_started;
# lifecycle_verify.  A deploy/setup recovery is deliberately the only retry.

LIFECYCLE_CLEANUP_FAILED=false
LIFECYCLE_LOADER_STARTED=false
LIFECYCLE_SETUP_ATTEMPTS=0
LIFECYCLE_DEPLOY_INVOCATIONS=0
LIFECYCLE_PRECLEAN_STATUS=0
readonly LIFECYCLE_CLEANUP_ABORT=70

lifecycle_write_archived_output_checksums() {
    local directory=$1 checksum_file=archived-output-checksums.csv
    (
        cd "$directory"
        printf 'path,sha256\n' > "$checksum_file"
        while IFS= read -r -d '' path; do
            printf '%s,%s\n' "$path" "$(sha256sum "$path" | awk '{print $1}')" >> "$checksum_file"
        done < <(find . -type f ! -name manifest.txt ! -name "$checksum_file" -printf '%P\0' | LC_ALL=C sort -z)
    )
}

lifecycle_archived_output_matches() {
    local directory=$1 row path expected actual count=0
    [[ -s "$directory/archived-output-checksums.csv" ]] || return 1
    IFS= read -r row < "$directory/archived-output-checksums.csv"
    [[ "$row" == 'path,sha256' ]] || return 1
    while IFS=, read -r path expected; do
        [[ "$expected" =~ ^[0-9a-f]{64}$ && -n "$path" && -f "$directory/$path" ]] || return 1
        actual=$(sha256sum "$directory/$path" | awk '{print $1}')
        [[ "$actual" == "$expected" ]] || return 1
        ((count+=1))
    done < <(tail -n +2 "$directory/archived-output-checksums.csv")
    ((count > 0))
}

lifecycle_manifest_value() {
    local manifest=$1 key=$2
    awk -F= -v key="$key" '
        $1 == key {print substr($0, length(key) + 2); found=1; exit}
        END {if (!found) exit 1}
    ' "$manifest"
}

lifecycle_success_manifest_matches() {
    local manifest=$1 setup_attempts deploy_attempts deploy_invocations loader_started cleanup_status lifecycle_phase
    setup_attempts=$(lifecycle_manifest_value "$manifest" setup_attempts) || return 1
    deploy_attempts=$(lifecycle_manifest_value "$manifest" deploy_attempts) || return 1
    deploy_invocations=$(lifecycle_manifest_value "$manifest" deploy_invocations) || return 1
    loader_started=$(lifecycle_manifest_value "$manifest" loader_started) || return 1
    cleanup_status=$(lifecycle_manifest_value "$manifest" cleanup_exit_status) || return 1
    lifecycle_phase=$(lifecycle_manifest_value "$manifest" lifecycle_phase) || return 1
    [[ "$setup_attempts" =~ ^[12]$ && "$deploy_invocations" =~ ^[12]$ ]] || return 1
    [[ "$deploy_attempts" == "$deploy_invocations" && "$loader_started" == true && "$cleanup_status" == 0 ]] || return 1
    [[ "$lifecycle_phase" == final ]] || return 1
    ((deploy_invocations <= setup_attempts))
}

# A pending cell is cleaned immediately before setup.  This is deliberately
# separate from acquisition: cleanup may be retried by the caller, but loader
# and acquisition are never replayed after lifecycle_run starts.
lifecycle_preclean() {
    local status
    LIFECYCLE_PRECLEAN_STATUS=0
    set +e
    lifecycle_cleanup pre-cell
    status=$?
    set -e
    LIFECYCLE_PRECLEAN_STATUS=$status
    if ((status != 0)); then
        LIFECYCLE_CLEANUP_FAILED=true
        return "$LIFECYCLE_CLEANUP_ABORT"
    fi
    return 0
}

# A cell always receives cleanup.  Only setup/deploy may be retried, once,
# because lifecycle_run begins loader/acquisition and must never be replayed.
lifecycle_execute() {
    local attempt=1 status=0 cleanup_status=0 run_status=0
    LIFECYCLE_CLEANUP_FAILED=false
    LIFECYCLE_LOADER_STARTED=false
    LIFECYCLE_SETUP_ATTEMPTS=0
    LIFECYCLE_DEPLOY_INVOCATIONS=0
    set +e
    while ((attempt <= 2)); do
        LIFECYCLE_SETUP_ATTEMPTS=$attempt
        lifecycle_setup "$attempt"
        status=$?
        if ((status == 0)); then
            ((LIFECYCLE_DEPLOY_INVOCATIONS+=1))
            lifecycle_deploy "$attempt"
            status=$?
        fi
        if ((status == 0)); then
            break
        fi

        lifecycle_cleanup "recovery-$attempt"
        cleanup_status=$?
        if ((cleanup_status != 0)); then
            LIFECYCLE_CLEANUP_FAILED=true
            lifecycle_finalize "$status" "$cleanup_status" "$LIFECYCLE_SETUP_ATTEMPTS" "$LIFECYCLE_DEPLOY_INVOCATIONS" false
            set -e
            return "$LIFECYCLE_CLEANUP_ABORT"
        fi
        if ((attempt == 2)); then
            lifecycle_finalize "$status" 0 "$LIFECYCLE_SETUP_ATTEMPTS" "$LIFECYCLE_DEPLOY_INVOCATIONS" false
            set -e
            return "$status"
        fi
        ((attempt+=1))
    done

    LIFECYCLE_LOADER_STARTED=true
    lifecycle_run
    run_status=$?
    lifecycle_cleanup final
    cleanup_status=$?
    lifecycle_finalize "$run_status" "$cleanup_status" "$LIFECYCLE_SETUP_ATTEMPTS" "$LIFECYCLE_DEPLOY_INVOCATIONS" true
    if ((cleanup_status != 0)); then
        LIFECYCLE_CLEANUP_FAILED=true
        set -e
        return "$LIFECYCLE_CLEANUP_ABORT"
    fi
    if ((run_status != 0)); then
        set -e
        return "$run_status"
    fi
    lifecycle_verify
    status=$?
    set -e
    return "$status"
}
