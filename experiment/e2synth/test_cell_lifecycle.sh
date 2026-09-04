#!/usr/bin/env bash
set -euo pipefail
source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd -P)/scripts/util/cell_lifecycle.sh"

reset_counts() { setup=0 deploy=0 run=0 clean=0 final=0 verify=0; }
assert_eq() { [[ "$1" == "$2" ]] || { echo "expected $2, got $1" >&2; exit 1; }; }

# One setup/deploy recovery is permitted before loader starts.
reset_counts
lifecycle_setup() { ((setup+=1)); [[ "$1" == 2 ]]; }
lifecycle_deploy() { ((deploy+=1)); }
lifecycle_run() { ((run+=1)); LIFECYCLE_ACQUISITION_STARTED=true; }
lifecycle_cleanup() { ((clean+=1)); }
lifecycle_finalize() { ((final+=1)); }
lifecycle_verify() { ((verify+=1)); }
lifecycle_execute
assert_eq "$setup,$deploy,$run,$final,$verify" "2,1,1,1,1"

# A pre-marker admission failure may rerun setup once.
reset_counts
lifecycle_setup() { ((setup+=1)); }
lifecycle_deploy() { ((deploy+=1)); }
lifecycle_run() { ((run+=1)); [[ "$run" == 1 ]] && { LIFECYCLE_ACQUISITION_STARTED=false; return 1; }; LIFECYCLE_ACQUISITION_STARTED=true; }
lifecycle_cleanup() { ((clean+=1)); }
lifecycle_finalize() { ((final+=1)); }
lifecycle_verify() { ((verify+=1)); }
lifecycle_execute
assert_eq "$setup,$deploy,$run,$final,$verify" "2,2,2,1,1"

# A post-marker acquisition failure is finalized and never replayed.
reset_counts
lifecycle_setup() { ((setup+=1)); }
lifecycle_deploy() { ((deploy+=1)); }
lifecycle_run() { ((run+=1)); LIFECYCLE_ACQUISITION_STARTED=true; return 9; }
lifecycle_cleanup() { ((clean+=1)); }
lifecycle_finalize() { ((final+=1)); }
lifecycle_verify() { ((verify+=1)); }
if lifecycle_execute; then status=0; else status=$?; fi
assert_eq "$status" 9
assert_eq "$setup,$deploy,$run,$final,$verify" "1,1,1,1,0"

# Cleanup contamination aborts the campaign with the shared sentinel.
reset_counts
lifecycle_setup() { ((setup+=1)); }
lifecycle_deploy() { ((deploy+=1)); }
lifecycle_run() { ((run+=1)); LIFECYCLE_ACQUISITION_STARTED=true; }
lifecycle_cleanup() { ((clean+=1)); [[ "$1" != final ]]; }
lifecycle_finalize() { ((final+=1)); }
lifecycle_verify() { ((verify+=1)); }
if lifecycle_execute; then status=0; else status=$?; fi
assert_eq "$status" "$LIFECYCLE_CLEANUP_ABORT"
assert_eq "$LIFECYCLE_CLEANUP_FAILED" true
assert_eq "$setup,$deploy,$run,$final,$verify" "1,1,1,1,0"

echo "E2-Synth cell lifecycle tests passed"
