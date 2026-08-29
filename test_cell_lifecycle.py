import subprocess
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parent
HELPER = ROOT / "scripts" / "util" / "cell_lifecycle.sh"


class CellLifecycleTest(unittest.TestCase):
    def run_script(self, script: str):
        return subprocess.run(
            ["bash", "-ceu", script], cwd=ROOT, text=True, capture_output=True
        )

    def test_terminal_b0_failure_cleans_continues_and_exits_nonzero(self):
        with tempfile.TemporaryDirectory() as directory:
            result = self.run_script(
                f'''source "{HELPER}"
root="{directory}"
suite_failed=false
for cell in b0 n4 n5 workload; do
  lifecycle_setup() {{ :; }}
  lifecycle_deploy() {{ :; }}
  lifecycle_run() {{
    printf 'run=%s\n' "$cell"
    [[ "$cell" != b0 ]]
  }}
  lifecycle_cleanup() {{ printf 'cleanup=%s:%s\n' "$cell" "$1"; }}
  lifecycle_finalize() {{
    mkdir -p "$root/$cell"
    printf 'cell=%s status=%s\n' "$cell" "$1" > "$root/$cell/output.txt"
    lifecycle_write_archived_output_checksums "$root/$cell"
  }}
  lifecycle_verify() {{ lifecycle_archived_output_matches "$root/$cell"; }}
  if lifecycle_execute; then :; else suite_failed=true; fi
done
printf 'suite_failed=%s\n' "$suite_failed"
[[ "$suite_failed" == true ]] || exit 1
[[ -f "$root/b0/archived-output-checksums.csv" ]]
[[ -f "$root/n4/archived-output-checksums.csv" ]]
[[ -f "$root/n5/archived-output-checksums.csv" ]]
printf 'tampered\n' >> "$root/n4/output.txt"
if lifecycle_archived_output_matches "$root/n4"; then exit 1; fi
exit 1
'''
            )
        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        self.assertIn("run=b0", result.stdout)
        self.assertIn("run=n4", result.stdout)
        self.assertIn("run=n5", result.stdout)
        self.assertIn("run=workload", result.stdout)
        self.assertIn("cleanup=b0:final", result.stdout)
        self.assertIn("suite_failed=true", result.stdout)

    def test_deploy_recovery_is_bounded_before_loader_only(self):
        result = self.run_script(
            f'''source "{HELPER}"
deploys=0
setups=0
loaders=0
cleanups=0
lifecycle_setup() {{ ((setups+=1)); printf 'setup=%s\n' "$1"; }}
lifecycle_deploy() {{ ((deploys+=1)); [[ "$deploys" -eq 2 ]]; }}
lifecycle_run() {{ ((loaders+=1)); return 1; }}
lifecycle_cleanup() {{ ((cleanups+=1)); }}
lifecycle_finalize() {{ printf 'finalize_setup_attempts=%s deploy_invocations=%s\n' "$3" "$4"; }}
lifecycle_verify() {{ :; }}
if lifecycle_execute; then exit 1; fi
printf 'setups=%s deploys=%s loaders=%s cleanups=%s started=%s\n' "$setups" "$deploys" "$loaders" "$cleanups" "$LIFECYCLE_LOADER_STARTED"
[[ "$setups" -eq 2 && "$deploys" -eq 2 && "$loaders" -eq 1 && "$cleanups" -eq 2 ]]
[[ "$LIFECYCLE_LOADER_STARTED" == true ]]
'''
        )
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("setup=1", result.stdout)
        self.assertIn("setup=2", result.stdout)
        self.assertIn("finalize_setup_attempts=2 deploy_invocations=2", result.stdout)
        self.assertIn("setups=2 deploys=2 loaders=1 cleanups=2 started=true", result.stdout)

    def test_preclean_precedes_acquisition_and_is_not_retried(self):
        result = self.run_script(
            f'''source "{HELPER}"
root="$(mktemp -d)"
events="$root/events"
record() {{ printf '%s\\n' "$1" >> "$events"; }}
lifecycle_setup() {{ record setup; }}
lifecycle_deploy() {{ record deploy; }}
lifecycle_run() {{ record run; }}
lifecycle_cleanup() {{ record "cleanup:$1"; }}
lifecycle_finalize() {{ record finalize; }}
lifecycle_verify() {{ :; }}
lifecycle_preclean
lifecycle_execute
expected=$'cleanup:pre-cell\\nsetup\\ndeploy\\nrun\\ncleanup:final\\nfinalize\\n'
actual=$(cat "$events")
[[ "$actual" == "${{expected%$'\\n'}}" ]]
[[ $(grep -c '^run$' "$events") -eq 1 ]]
'''
        )
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_preclean_failure_exposes_actual_cleanup_status_for_immutable_archive(self):
        result = self.run_script(
            f'''source "{HELPER}"
lifecycle_cleanup() {{ return 23; }}
if lifecycle_preclean; then exit 1; else status=$?; fi
[[ "$status" -eq "$LIFECYCLE_CLEANUP_ABORT" ]]
[[ "$LIFECYCLE_CLEANUP_FAILED" == true ]]
[[ "$LIFECYCLE_PRECLEAN_STATUS" -eq 23 ]]
'''
        )
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_runners_archive_create_only_initial_cleanup_evidence(self):
        for runner in ("run_rps_per_workload.sh", "run_trace_ablation.sh"):
            with self.subTest(runner=runner):
                source = (ROOT / runner).read_text(encoding="utf-8")
                self.assertIn("initial_cleanup_matches()", source)
                self.assertIn('mkdir -- "$destination"', source)
                self.assertIn('archived_output_matches "$destination"', source)
                self.assertIn('run_initial_cleanup ||', source)
                self.assertIn("if lifecycle_execute; then", source)
                self.assertIn("else\n        status=$?", source)
                self.assertNotIn("fi\n    local status=$?", source)
                self.assertIn(
                    'lifecycle_finalize "$status" "$LIFECYCLE_PRECLEAN_STATUS" 0 0 false',
                    source,
                )
                self.assertIn("immutable evidence retained at $destination", source)

    def test_evaluation_cell_make_target_is_local_only(self):
        source = (ROOT / "Makefile").read_text(encoding="utf-8")
        target = source.split("clean-evaluation-cell:", 1)[1].split("\n\n", 1)[0]
        self.assertIn("kn service delete --all", target)
        self.assertIn("kubectl delete --all deployments,replicasets,pods,jobs,podautoscalers -n default", target)
        self.assertNotIn("kubectl delete --all all", target)
        self.assertIn("reset_kn_global.sh", target)
        self.assertIn("rm -f loader", target)
        for forbidden in ("clean_prometheus.sh", "activator", "go mod tidy"):
            self.assertNotIn(forbidden, target)

    def test_success_manifest_requires_bounded_attempts_and_clean_teardown(self):
        with tempfile.TemporaryDirectory() as directory:
            manifest = Path(directory) / "manifest.txt"
            manifest.write_text(
                "setup_attempts=2\n"
                "deploy_attempts=1\n"
                "deploy_invocations=1\n"
                "loader_started=true\n"
                "cleanup_exit_status=0\n"
                "lifecycle_phase=final\n",
                encoding="utf-8",
            )
            result = self.run_script(
                f'''source "{HELPER}"
lifecycle_success_manifest_matches "{manifest}"
'''
            )
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            manifest.write_text(
                "setup_attempts=1\n"
                "deploy_attempts=2\n"
                "deploy_invocations=2\n"
                "loader_started=true\n"
                "cleanup_exit_status=0\n"
                "lifecycle_phase=final\n",
                encoding="utf-8",
            )
            result = self.run_script(
                f'''source "{HELPER}"
if lifecycle_success_manifest_matches "{manifest}"; then exit 1; fi
'''
            )
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)


if __name__ == "__main__":
    unittest.main()
