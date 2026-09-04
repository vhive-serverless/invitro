#!/usr/bin/env bash
set -euo pipefail

loader_ip=${1:?loader IP is required}
staged_keys=${2:?staged authoritative key file is required}
ssh_dir=${E2_SYNTH_SSH_DIR:-$HOME/.ssh}
if [[ "$staged_keys" != /* ]]; then staged_keys="$HOME/$staged_keys"; fi
[[ "$loader_ip" =~ ^[0-9A-Fa-f:.]+$ ]] || { echo "invalid Loader IP" >&2; exit 2; }
[[ -s "$staged_keys" ]] || { echo "missing authoritative Loader keys" >&2; exit 2; }

mkdir -p -- "$ssh_dir"
chmod 700 -- "$ssh_dir"
known_hosts="$ssh_dir/known_hosts"
temporary=$(mktemp "$ssh_dir/.known_hosts.e2-synth.XXXXXX")
cleanup() { rm -f -- "$temporary" "$temporary.old" "$staged_keys"; }
trap cleanup EXIT
if [[ -f "$known_hosts" ]]; then cp -- "$known_hosts" "$temporary"; fi
ssh-keygen -q -f "$temporary" -R "$loader_ip" >/dev/null 2>&1 || true
awk -v host="$loader_ip" '
    NF != 3 || $1 != host || ($2 !~ /^ssh-/ && $2 !~ /^ecdsa-/) {exit 1}
' "$staged_keys"
cat -- "$staged_keys" >> "$temporary"
chmod 600 -- "$temporary"
mv -f -- "$temporary" "$known_hosts"

if [[ "${E2_SYNTH_SKIP_PROBE:-false}" != true ]]; then
    ssh -o BatchMode=yes -o StrictHostKeyChecking=yes -o ConnectTimeout=5 "$loader_ip" true
    rsync --dry-run -e 'ssh -o BatchMode=yes -o StrictHostKeyChecking=yes -o ConnectTimeout=5' \
        /dev/null "$loader_ip:/tmp/" >/dev/null
fi
printf '%s\n' 'known_hosts_status=PASS ssh_status=PASS rsync_dry_run_status=PASS'
