#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd -P)
source "$repo_root/scripts/util/e2_synth_runner_policy.sh"
temporary=$(mktemp -d)
cleanup() { rm -rf -- "$temporary"; }
trap cleanup EXIT

mkdir -p "$temporary/unacquired" "$temporary/acquired" "$temporary/legacy-nested" "$temporary/manifest-acquired"
touch "$temporary/acquired/acquisition-started.marker"
mkdir -p "$temporary/legacy-nested/out"
touch "$temporary/legacy-nested/out/acquisition-started.marker"
printf 'acquisition_started=true\n' > "$temporary/manifest-acquired/manifest.txt"
! e2_synth_acquisition_started "$temporary/unacquired"
e2_synth_acquisition_started "$temporary/acquired"
e2_synth_acquisition_started "$temporary/legacy-nested"
e2_synth_acquisition_started "$temporary/manifest-acquired"

namespace_a=$(e2_synth_scratch_namespace current /tmp/result-a)
namespace_a_again=$(e2_synth_scratch_namespace current /tmp/result-a)
namespace_b=$(e2_synth_scratch_namespace current /tmp/result-b)
namespace_other_cluster=$(e2_synth_scratch_namespace supplied /tmp/result-a)
[[ "$namespace_a" =~ ^[0-9a-f]{64}$ && "$namespace_a" == "$namespace_a_again" ]]
[[ "$namespace_a" != "$namespace_b" && "$namespace_a" != "$namespace_other_cluster" ]]

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

# RDMA payload staging is E2-Synth-specific, prepares the storage root first,
# then synchronizes all three declared input roots in deterministic order.
mkdir -p "$temporary/stage-bin" "$temporary/khala/assets/nexus-benchmark-payload/input_payload" \
    "$temporary/khala/assets/nexus-benchmark-payload/test" "$temporary/khala/assets/synthetic-payload"
printf '{"storage_nodes":["storage-a","storage-b"]}\n' > "$temporary/worker-node.json"
printf '%s\n' '#!/usr/bin/env bash' 'printf "ssh %s\\n" "$*" >> "$E2_SYNTH_TEST_CALLS"' > "$temporary/stage-bin/ssh"
printf '%s\n' '#!/usr/bin/env bash' 'printf "rsync %s\\n" "$*" >> "$E2_SYNTH_TEST_CALLS"' > "$temporary/stage-bin/rsync"
chmod +x "$temporary/stage-bin/ssh" "$temporary/stage-bin/rsync"
E2_SYNTH_TEST_CALLS="$temporary/stage-calls" PATH="$temporary/stage-bin:$PATH" \
    e2_synth_stage_rdma_payloads "$temporary/khala" "$temporary/worker-node.json"
[[ "$(grep -c '^ssh ' "$temporary/stage-calls")" == 2 ]]
[[ "$(grep -c '^rsync ' "$temporary/stage-calls")" == 6 ]]
[[ "$(sed -n '1p' "$temporary/stage-calls")" == *'storage-a'* ]]
[[ "$(sed -n '2p' "$temporary/stage-calls")" == *'storage-a:rdma-demo/assets/nexus-benchmark-payload/input_payload/'* ]]
[[ "$(sed -n '4p' "$temporary/stage-calls")" == *'storage-a:rdma-demo/assets/synthetic-payload-input/'* ]]
[[ "$(sed -n '5p' "$temporary/stage-calls")" == *'storage-b'* ]]
! e2_synth_stage_rdma_payloads "$temporary/missing" "$temporary/worker-node.json"
printf '{"storage_nodes":[]}\n' > "$temporary/no-storage.json"
! e2_synth_stage_rdma_payloads "$temporary/khala" "$temporary/no-storage.json"

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
