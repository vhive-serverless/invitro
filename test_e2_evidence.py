import csv
import json
import subprocess
import tempfile
import unittest
from pathlib import Path


class E2EvidenceTest(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.root = Path(self.directory.name)
        self.prefix = self.root / "experiment"
        self.worker_config = self.root / "worker.json"
        self.worker_config.write_text('{"worker_nodes":["10.0.1.3"]}\n', encoding="utf-8")
        self.loader_log = self.root / "loader.log"
        self.loader_log.write_text("loader completed\n", encoding="utf-8")
        self.validator = Path(__file__).with_name("experiment") / "e2" / "validate_evidence.py"
        self._write_csv("_duration_3.csv", ["success"], [["true"]])
        (self.root / "experiment_cluster_usage_3.csv").write_text(
            json.dumps({"cpu": ["1n"], "hardware_metrics": {"worker": {"cgroups": {}}}}) + "\n",
            encoding="utf-8",
        )
        self._write_csv(
            "_kn_stats_3.csv",
            ["desired_pods", "unready_pods", "pending_pods", "requested_pods", "running_pods"],
            [["0", "0", "0", "2", "2"]],
        )
        self._write_csv("_deployment_scale_3.csv", ["function", "running_pods"], [["helloworld-0", "2"]])
        for suffix, content in (
            ("_perf_0.csv", b"cycles,1\n"),
            ("_perf_0.data", b"perf-data"),
            ("_perf_0.svg", b"<svg></svg>"),
            ("_perf_filtered_0.svg", b"<svg></svg>"),
        ):
            Path(str(self.prefix) + suffix).write_bytes(content)

    def _write_csv(self, suffix, fields, rows):
        with Path(str(self.prefix) + suffix).open("w", newline="", encoding="utf-8") as handle:
            writer = csv.writer(handle)
            writer.writerow(fields)
            writer.writerows(rows)

    def _run(self, perf="true"):
        return subprocess.run(
            [
                "python3", str(self.validator), "--output-prefix", str(self.prefix),
                "--loader-log", str(self.loader_log), "--worker-config", str(self.worker_config),
                "--perf-enabled", perf,
            ],
            text=True,
            capture_output=True,
        )

    def test_accepts_complete_metrics_and_four_perf_artifacts(self):
        result = self._run()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("evidence_status=PASS", result.stdout)
        self.assertIn("perf_artifact_count=4", result.stdout)

    def test_rejects_missing_perf_artifact(self):
        Path(str(self.prefix) + "_perf_0.data").unlink()
        result = self._run()
        self.assertEqual(result.returncode, 1)
        self.assertIn("missing or empty perf artifact", result.stdout)

    def test_rejects_prometheus_transport_error(self):
        self.loader_log.write_text("Fail to parse cluster usage: error querying prometheus\n", encoding="utf-8")
        result = self._run()
        self.assertEqual(result.returncode, 1)
        self.assertIn("telemetry transport/parser errors", result.stdout)

    def test_rejects_function_deployment_failure(self):
        self.loader_log.write_text(
            "Failed to deploy function helloworld: exit status 1\n", encoding="utf-8"
        )
        result = self._run()
        self.assertEqual(result.returncode, 1)
        self.assertIn("loader recorded function deployment failure", result.stdout)

    def test_rejects_sentinel_only_cluster_metrics(self):
        (self.root / "experiment_cluster_usage_3.csv").write_text(
            json.dumps({"cpu": None, "hardware_metrics": None}) + "\n", encoding="utf-8"
        )
        result = self._run(perf="false")
        self.assertEqual(result.returncode, 1)
        self.assertIn("no hardware-manager sample", result.stdout)

    def test_runner_cleanup_has_phase_snapshot_policy(self):
        source = Path(__file__).with_name("run_rps_per_workload.sh").read_text(encoding="utf-8")
        self.assertIn("--remove-snapshots=true", source)
        self.assertIn('pre-cell)', source)
        self.assertIn('recovery-1|recovery-2)', source)
        self.assertIn('policy=invalidate-stale-scratch', source)
        self.assertIn('policy=invalidate', source)
        self.assertIn('policy=preserve', source)
        self.assertIn('snapshot_cleanup_policy=', source)


if __name__ == "__main__":
    unittest.main()
