#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd -P)
source "$repo_root/scripts/util/e2_synth_runner_policy.sh"
temporary=$(mktemp -d)
cleanup() { rm -rf -- "$temporary"; }
trap cleanup EXIT

mkdir -p "$temporary/unacquired" "$temporary/acquired"
touch "$temporary/acquired/acquisition-started.marker"
! e2_synth_acquisition_started "$temporary/unacquired"
e2_synth_acquisition_started "$temporary/acquired"

# The outer campaign must not reach a second cell after an acquired failure.
deployed=0
for cell in "$temporary/acquired" "$temporary/unacquired"; do
    ((deployed+=1))
    if e2_synth_acquisition_started "$cell"; then break; fi
done
[[ "$deployed" == 1 ]]

# Immutable seed verification derives from the declared all-zero workload,
# not from the mutable staging directory rewritten by cluster cleanup.
truncate -s 65536 "$temporary/zero-65536"
seed_digest=$(e2_synth_validate_zero_payload "$temporary/zero-65536" 65536)
[[ "$seed_digest" == de2f256064a0af797747c2b97505dc0b9f3df0de4f489eac731c23ae9ca9cc31 ]]
rm -f -- "$temporary/zero-65536"
[[ "$(e2_synth_zero_payload_digest 65536)" == "$seed_digest" ]]
truncate -s 65535 "$temporary/wrong-size"
! e2_synth_validate_zero_payload "$temporary/wrong-size" 65536
truncate -s 65536 "$temporary/nonzero"
printf '\1' | dd of="$temporary/nonzero" bs=1 seek=0 conv=notrunc status=none
! e2_synth_validate_zero_payload "$temporary/nonzero" 65536
ln -s "$temporary/nonzero" "$temporary/symlink"
! e2_synth_validate_zero_payload "$temporary/symlink" 65536

ssh-keygen -q -t ed25519 -N '' -f "$temporary/old" >/dev/null
ssh-keygen -q -t ed25519 -N '' -f "$temporary/new" >/dev/null
mkdir -p "$temporary/ssh"
{
    printf '192.0.2.10 '
    cut -d' ' -f1,2 "$temporary/old.pub"
    printf 'unrelated.example '
    cut -d' ' -f1,2 "$temporary/old.pub"
} > "$temporary/ssh/known_hosts"
{
    printf '192.0.2.10 '
    cut -d' ' -f1,2 "$temporary/new.pub"
} > "$temporary/staged"
E2_SYNTH_SSH_DIR="$temporary/ssh" E2_SYNTH_SKIP_PROBE=true \
    bash "$repo_root/scripts/util/e2_synth_install_authoritative_host_keys.sh" \
    192.0.2.10 "$temporary/staged" > "$temporary/probe"
grep -Fqx 'known_hosts_status=PASS ssh_status=PASS rsync_dry_run_status=PASS' "$temporary/probe"
grep -Fq 'unrelated.example ' "$temporary/ssh/known_hosts"
grep -Fq "192.0.2.10 $(cut -d' ' -f1,2 "$temporary/new.pub")" "$temporary/ssh/known_hosts"
! grep -Fq "192.0.2.10 $(cut -d' ' -f1,2 "$temporary/old.pub")" "$temporary/ssh/known_hosts"

# The real helper is sent through `ssh ... bash -s`; nested transport probes
# must not consume the remaining streamed script from stdin.
mkdir -p "$temporary/bin" "$temporary/stream-ssh"
printf '%s\n' \
    '#!/usr/bin/env bash' \
    'case " $* " in' \
    "    *' -n '*) exit 0 ;;" \
    '    *) cat >/dev/null; exit 0 ;;' \
    'esac' > "$temporary/bin/ssh"
printf '%s\n' '#!/usr/bin/env bash' 'cat >/dev/null' 'exit 0' > "$temporary/bin/rsync"
chmod +x "$temporary/bin/ssh" "$temporary/bin/rsync"
{
    printf '192.0.2.10 '
    cut -d' ' -f1,2 "$temporary/new.pub"
} > "$temporary/staged-stream"
PATH="$temporary/bin:$PATH" E2_SYNTH_SSH_DIR="$temporary/stream-ssh" \
    bash -s -- 192.0.2.10 "$temporary/staged-stream" \
    < "$repo_root/scripts/util/e2_synth_install_authoritative_host_keys.sh" \
    > "$temporary/stream-probe"
grep -Fqx 'known_hosts_status=PASS ssh_status=PASS rsync_dry_run_status=PASS' "$temporary/stream-probe"

echo 'E2-Synth runner policy tests passed'
