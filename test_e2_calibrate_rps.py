import csv
import os
import subprocess
import tempfile
import unittest
from pathlib import Path

from e2_calibrate_rps import (
    FAILURE_THRESHOLD,
    STEPS,
    WORKLOADS,
    analyze,
    build_plan,
    observe_duration,
    read_averages,
    write_fixed_trace,
    write_trace,
)


class CalibrationTest(unittest.TestCase):
    def setUp(self):
        self.averages = {workload: 10.0 + index for index, workload in enumerate(WORKLOADS)}
        self.plans = build_plan(self.averages, 28)

    def _observations(self, failure_step=None, *, threshold_kind="p99"):
        rows = []
        for workload in WORKLOADS:
            plan = self.plans[workload]
            threshold = 5.0 * float(plan["unloaded_average_ms"])
            for step, rps in enumerate(plan["levels"], 1):
                p99 = threshold - 0.001
                failure_rate = FAILURE_THRESHOLD
                if workload == "helloworld" and step == failure_step:
                    if threshold_kind == "p99":
                        p99 = threshold
                    else:
                        failure_rate = FAILURE_THRESHOLD + 0.001
                rows.append({
                    "workload": workload,
                    "step": str(step),
                    "rps": str(rps),
                    "failure_rate": str(failure_rate),
                    "p99_ms": str(p99),
                })
        return rows

    def test_plan_has_twenty_distinct_linear_levels(self):
        for workload, plan in self.plans.items():
            self.assertEqual(len(plan["levels"]), STEPS, workload)
            self.assertEqual(len(set(plan["levels"])), STEPS, workload)
            self.assertEqual(plan["levels"][-1], plan["rbound"], workload)

    def test_explicit_larger_ceiling_doubles_rbound_without_auto_extension(self):
        extended = build_plan(self.averages, 28, 2.0)
        for workload in WORKLOADS:
            self.assertEqual(extended[workload]["rbound"], int(28 * 1000 * 2 / self.averages[workload]))
            self.assertGreater(extended[workload]["rbound"], self.plans[workload]["rbound"])
            self.assertEqual(extended[workload]["ceiling_multiplier"], 2.0)

    def test_exact_slo_boundary_uses_previous_level(self):
        result = {row["workload"]: row for row in analyze(self.plans, self._observations(2))}
        hello = result["helloworld"]
        self.assertEqual(hello["status"], "BOUNDARY_OBSERVED")
        self.assertEqual(hello["first_failing_step"], "2")
        self.assertEqual(hello["rmax_b0"], self.plans["helloworld"]["levels"][0])
        self.assertEqual(hello["rref"], self.plans["helloworld"]["levels"][0] // 2)

    def test_failure_rate_break_is_strictly_greater_than_five_percent(self):
        result = {row["workload"]: row for row in analyze(
            self.plans, self._observations(3, threshold_kind="failure")
        )}
        self.assertEqual(result["helloworld"]["first_failing_step"], "3")

    def test_no_observed_boundary_is_right_censored(self):
        result = {row["workload"]: row for row in analyze(self.plans, self._observations())}
        self.assertTrue(all(row["status"] == "RIGHT_CENSORED" for row in result.values()))
        self.assertTrue(all(row["reference_kind"] == "RIGHT_CENSORED_REFERENCE" for row in result.values()))
        for workload, row in result.items():
            self.assertEqual(row["rref"], self.plans[workload]["levels"][-1] // 2)
            self.assertEqual(row["rmax_b0"], "")

    def test_e1_summary_rejects_missing_and_duplicate_workloads(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "averages.csv"
            with path.open("w", newline="", encoding="utf-8") as handle:
                writer = csv.DictWriter(handle, fieldnames=["workload", "unloaded_average_ms", "n_samples"])
                writer.writeheader()
                for workload in WORKLOADS[:-1]:
                    writer.writerow({"workload": workload, "unloaded_average_ms": 10, "n_samples": 5})
            with self.assertRaisesRegex(ValueError, "workload set mismatch"):
                read_averages(path)
            with path.open("a", newline="", encoding="utf-8") as handle:
                writer = csv.DictWriter(handle, fieldnames=["workload", "unloaded_average_ms", "n_samples"])
                writer.writerow({"workload": WORKLOADS[0], "unloaded_average_ms": 10, "n_samples": 5})
            with self.assertRaisesRegex(ValueError, "duplicate"):
                read_averages(path)

    def test_trace_contains_two_warmup_and_twenty_measurement_minutes(self):
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory) / "trace"
            write_trace(self.plans["helloworld"], "helloworld", output, 2)
            with (output / "invocations.csv").open(newline="", encoding="utf-8") as handle:
                rows = list(csv.DictReader(handle))
                self.assertEqual(len(rows), 1)
                self.assertEqual(list(rows[0])[1:], ["-2", "-1", *map(str, range(1, 21))])
                self.assertEqual(int(rows[0]["20"]), self.plans["helloworld"]["levels"][-1] * 60)

    def test_observer_uses_execution_phase_and_microseconds(self):
        with tempfile.TemporaryDirectory() as directory:
            duration = Path(directory) / "duration.csv"
            with duration.open("w", newline="", encoding="utf-8") as handle:
                writer = csv.DictWriter(handle, fieldnames=["phase", "invocationID", "responseTime", "success"])
                writer.writeheader()
                writer.writerow({"phase": 1, "invocationID": "min0.inv0", "responseTime": 999000, "success": True})
                for minute in range(2, 22):
                    writer.writerow({"phase": 2, "invocationID": f"min{minute}.inv0", "responseTime": 12500, "success": True})
            rows = observe_duration(duration, "helloworld", self.plans["helloworld"])
            self.assertEqual(len(rows), 20)
            self.assertTrue(all(row["p99_ms"] == 12.5 for row in rows))
            self.assertTrue(all(row["failure_rate"] == 0 for row in rows))

    def test_fixed_trace_maps_python_go_js_and_nexus_names(self):
        cases = {
            "invm-py": "helloworld",
            "invm-go": "gohelloworld",
            "invm-js": "jshelloworld",
            "hosttcp-go": "gohelloworld-s3-rpc-hosttcp",
            "nexus-go": "gohelloworld-s3-rpc-shmem",
            "nexus-py": "helloworld-s3-rpc-shmem",
            "nexus-js": "jshelloworld-s3-rpc-shmem",
            "nexus-rdma-py": "helloworld-s3-rpc-rdma",
        }
        with tempfile.TemporaryDirectory() as directory:
            for index, (mode, expected) in enumerate(cases.items()):
                output = Path(directory) / str(index)
                write_fixed_trace(10, "helloworld", mode, 7, output, 2, 3)
                with (output / "invocations.csv").open(newline="", encoding="utf-8") as handle:
                    self.assertEqual(next(csv.DictReader(handle))["FunctionName"], expected)

    def test_high_level_runner_dry_run_has_exact_frozen_matrix(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            averages = root / "averages.csv"
            with averages.open("w", newline="", encoding="utf-8") as handle:
                writer = csv.DictWriter(handle, fieldnames=["workload", "unloaded_average_ms", "n_samples"])
                writer.writeheader()
                for workload in WORKLOADS:
                    writer.writerow({"workload": workload, "unloaded_average_ms": self.averages[workload], "n_samples": 80})
            reference = root / "reference.csv"
            with reference.open("w", newline="", encoding="utf-8") as handle:
                fields = [
                    "workload", "unloaded_average_ms", "worker_cores", "ceiling_multiplier", "rbound",
                    "first_failing_step", "first_failing_rps", "rmax_b0", "rref", "status",
                ]
                writer = csv.DictWriter(handle, fieldnames=fields)
                writer.writeheader()
                for workload, plan in self.plans.items():
                    writer.writerow({
                        "workload": workload,
                        "unloaded_average_ms": plan["unloaded_average_ms"],
                        "worker_cores": 28,
                        "ceiling_multiplier": 1.0,
                        "rbound": plan["rbound"],
                        "first_failing_step": 2,
                        "first_failing_rps": plan["levels"][1],
                        "rmax_b0": plan["levels"][0],
                        "rref": max(1, plan["levels"][0] // 2),
                        "status": "BOUNDARY_OBSERVED",
                    })
            script = Path(__file__).with_name("run_rps_per_workload.sh")
            calibration = subprocess.run([
                str(script), "calibrate", "--profile", "4-node", "--e1-summary", str(averages),
                "--worker-cores", "28", "--slo-multiplier", "5", "--failure-threshold", "0.05",
                "--warmup-minutes", "2", "--steps", "20", "--minutes-per-step", "1",
                "--no-retry", "--result-root", str(root / "calibration"), "--dry-run",
            ], check=True, capture_output=True, text=True)
            self.assertEqual(sum(line.startswith("CELL ") for line in calibration.stdout.splitlines()), 11)
            collection = subprocess.run([
                str(script), "collect", "--profile", "4-node", "--reference", str(reference),
                "--e1-summary", str(averages), "--replicas", "320", "--repetitions", "1",
                "--result-root", str(root / "collection"), "--dry-run",
            ], check=True, capture_output=True, text=True)
            cells = [line for line in collection.stdout.splitlines() if line.startswith("CELL ")]
            self.assertEqual(len(cells), 38)
            self.assertEqual(sum("workload=helloworld " in line for line in cells), 8)
            self.assertIn("E2_DRY_RUN_READY", collection.stdout)
            self.assertFalse((root / "collection").exists())

    def test_runner_uses_archived_worker_config_not_the_checked_in_default(self):
        source = Path(__file__).with_name("run_rps_per_workload.sh").read_text(encoding="utf-8")
        self.assertIn("source /etc/profile", source)
        self.assertIn("EVAL_SCRATCH_ROOT:-/mnt/resources/nexus-evaluation/.scratch/e2", source)
        self.assertNotIn("data/traces/nexus-e2", source)
        self.assertNotIn("data/out/nexus-e2", source)
        self.assertIn('discover_cluster_topology "$result_root/cluster-inventory.txt" "$result_root/worker-node.json"', source)
        self.assertIn('--worker-config "$worker_config"', source)
        self.assertIn('worker_config_sha256=$(digest "$worker_config")', source)
        self.assertIn('remote_provenance_sha256=$(digest "$result_root/remote-provenance.txt")', source)
        self.assertIn('kubectl get nodes -o json | jq -S', source)
        self.assertIn('cp -- "$result_root/remote-provenance.txt" "$scratch_out/remote-provenance.txt"', source)
        self.assertIn('bin/nexus-backend', source)
        self.assertIn('bin/hardware-manager', source)
        self.assertIn('expected_invitro_head=$(git rev-parse HEAD)', source)
        self.assertIn('test "$head" = "$expected_head"', source)
        self.assertIn('local output=$1 worker_config=$2 require_rdma=$3', source)
        self.assertIn('snapshot_remote_provenance "$result_root/remote-provenance.txt" "$result_root/worker-node.json" "$require_reference"', source)
        self.assertIn('if [[ "$require_rdma" == true ]]; then', source)
        self.assertNotIn('--vm-config "$vm_config"', source)

    def test_runner_rejects_scratch_inside_worktree(self):
        script = Path(__file__).with_name("run_rps_per_workload.sh")
        environment = os.environ.copy()
        environment["EVAL_SCRATCH_ROOT"] = str(Path(__file__).resolve().parent / ".tmp" / "e2")
        result = subprocess.run(
            [str(script), "calibrate", "--dry-run"],
            capture_output=True,
            text=True,
            env=environment,
        )
        self.assertEqual(result.returncode, 2)
        self.assertIn("must be outside the InVitro worktree", result.stderr)


if __name__ == "__main__":
    unittest.main()
