import csv
import subprocess
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parent
WORKLOADS = (
    "helloworld", "chameleonserve", "cnnserve", "imageresize", "lrserving",
    "mapper", "pyaesserve", "reducer", "rnnserve", "streducer", "sttrainer",
)


class AblationDryRunTest(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.root = Path(self.temporary.name)
        self.reference = self.root / "b0-rps-reference.csv"
        with self.reference.open("w", newline="", encoding="utf-8") as handle:
            writer = csv.DictWriter(handle, fieldnames=(
                "workload", "unloaded_average_ms", "worker_cores", "rbound",
                "first_failing_step", "first_failing_rps", "rmax_b0", "rref", "status",
            ))
            writer.writeheader()
            for workload in WORKLOADS:
                writer.writerow({
                    "workload": workload,
                    "unloaded_average_ms": 10,
                    "worker_cores": 28,
                    "rbound": 2800,
                    "first_failing_step": 11,
                    "first_failing_rps": 1540,
                    "rmax_b0": 1400,
                    "rref": 700,
                    "status": "BOUNDARY_OBSERVED",
                })

    def tearDown(self):
        self.temporary.cleanup()

    def command(self, *extra):
        return [
            "bash", "run_trace_ablation.sh",
            "--profile", "4-node",
            "--modes", "invm-py,nexus-py,nexus-rdma-py",
            "--reference", str(self.reference),
            "--start-scale", "1",
            "--step", "1",
            "--end-scale", "1",
            "--warmup-minutes", "2",
            "--repetitions", "1",
            "--cooldown-seconds", "0",
            "--result-root", str(self.root / "result"),
            *extra,
        ]

    def test_four_node_dry_run_is_exact_and_side_effect_free(self):
        result = subprocess.run(
            self.command("--dry-run"), cwd=ROOT, check=True,
            capture_output=True, text=True,
        )
        output = result.stdout
        self.assertEqual(output.count("CELL experiment=e3-e4"), 3)
        for mode in ("invm-py", "nexus-py", "nexus-rdma-py"):
            self.assertIn(f"mode={mode}", output)
        self.assertIn("claim_bearing=false", output)
        self.assertIn("deployed_function_rows=10", output)
        self.assertIn("minio_endpoint=10.0.1.4:9001", output)
        self.assertIn("E3_E4_DRY_RUN_READY", output)
        self.assertFalse((self.root / "result").exists())

    def test_eighteen_node_profile_requires_explicit_claim_flag(self):
        command = self.command("--dry-run")
        command[command.index("4-node")] = "18-node"
        command[command.index("1", command.index("--end-scale"))] = "27"
        command[command.index("1", command.index("--repetitions"))] = "3"
        result = subprocess.run(command, cwd=ROOT, capture_output=True, text=True)
        self.assertEqual(result.returncode, 2)
        self.assertIn("requires explicit --claim-run", result.stderr)

        command.insert(-1, "--claim-run")
        result = subprocess.run(command, cwd=ROOT, check=True, capture_output=True, text=True)
        self.assertEqual(result.stdout.count("CELL experiment=e3-e4"), 9)
        self.assertIn("claim_bearing=true", result.stdout)
        self.assertIn("deployed_function_rows=270", result.stdout)
        self.assertIn("minio_endpoint=http://myminio-api.minio.10.200.3.4.sslip.io", result.stdout)

    def test_unknown_mode_fails_before_plan_or_side_effects(self):
        command = self.command("--dry-run")
        command[command.index("invm-py,nexus-py,nexus-rdma-py")] = "invm-py,nexus-py,unknown-mode"
        result = subprocess.run(command, cwd=ROOT, capture_output=True, text=True)
        self.assertEqual(result.returncode, 2)
        self.assertIn("missing E3/E4 mode nexus-rdma-py", result.stderr)
        self.assertNotIn("CELL experiment=e3-e4", result.stdout)
        self.assertFalse((self.root / "result").exists())


if __name__ == "__main__":
    unittest.main()
