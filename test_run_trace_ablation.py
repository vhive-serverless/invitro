import csv
import os
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
                "ceiling_multiplier", "first_failing_step", "first_failing_rps", "rmax_b0", "rref", "status",
            ))
            writer.writeheader()
            for workload in WORKLOADS:
                writer.writerow({
                    "workload": workload,
                    "unloaded_average_ms": 10,
                    "worker_cores": 28,
                    "rbound": 2800,
                    "ceiling_multiplier": 1.0,
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
        self.assertEqual(output.count("CELL experiment=e3"), 3)
        for mode in ("invm-py", "nexus-py", "nexus-rdma-py"):
            self.assertIn(f"mode={mode}", output)
        self.assertIn("claim_bearing=false", output)
        self.assertIn("deployed_function_rows=10", output)
        self.assertIn("minio_endpoint=myminio-api.minio.10.200.3.4.sslip.io:80", output)
        self.assertEqual(output.count("minio_route=istio"), 2)
        self.assertEqual(output.count("minio_route=rdma"), 1)
        self.assertIn("E3_DRY_RUN_READY", output)
        self.assertFalse((self.root / "result").exists())

    def test_eighteen_node_profile_requires_explicit_claim_flag(self):
        command = self.command("--dry-run")
        command[command.index("4-node")] = "18-node"
        command[command.index("1", command.index("--end-scale"))] = "27"
        result = subprocess.run(command, cwd=ROOT, capture_output=True, text=True)
        self.assertEqual(result.returncode, 2)
        self.assertIn("requires explicit --claim-run", result.stderr)

        command.insert(-1, "--claim-run")
        result = subprocess.run(command, cwd=ROOT, check=True, capture_output=True, text=True)
        self.assertEqual(result.stdout.count("CELL experiment=e3"), 3)
        self.assertIn("claim_bearing=true", result.stdout)
        self.assertIn("deployed_function_rows=270", result.stdout)
        self.assertIn("minio_endpoint=myminio-api.minio.10.200.3.4.sslip.io:80", result.stdout)
        self.assertEqual(result.stdout.count("minio_route=istio"), 2)
        self.assertEqual(result.stdout.count("minio_route=rdma"), 1)

    def test_eighteen_node_profile_rejects_trace_contract_override(self):
        command = self.command("--dry-run")
        command[command.index("4-node")] = "18-node"
        command[command.index("1", command.index("--end-scale"))] = "27"
        command.insert(-1, "--claim-run")
        command[-1:-1] = ["--shift-step", "9"]
        result = subprocess.run(command, cwd=ROOT, capture_output=True, text=True)
        self.assertEqual(result.returncode, 2)
        self.assertIn("requires SHIFT_STEP=10 and DIVISOR=100", result.stderr)

    def test_ten_and_fourteen_node_profiles_support_claim_dry_runs(self):
        for profile in ("10-node", "14-node"):
            with self.subTest(profile=profile):
                command = self.command("--dry-run")
                command[command.index("4-node")] = profile
                command[command.index("1", command.index("--end-scale"))] = "27"
                command.insert(-1, "--claim-run")
                result = subprocess.run(command, cwd=ROOT, check=True,
                                        capture_output=True, text=True)
                self.assertEqual(result.stdout.count("CELL experiment=e3"), 3)
                self.assertIn("claim_bearing=true", result.stdout)
                self.assertIn("deployed_function_rows=270", result.stdout)

    def test_unknown_mode_fails_before_plan_or_side_effects(self):
        command = self.command("--dry-run")
        command[command.index("invm-py,nexus-py,nexus-rdma-py")] = "invm-py,nexus-py,unknown-mode"
        result = subprocess.run(command, cwd=ROOT, capture_output=True, text=True)
        self.assertEqual(result.returncode, 2)
        self.assertIn("missing E3 mode nexus-rdma-py", result.stderr)
        self.assertNotIn("CELL experiment=e3", result.stdout)
        self.assertFalse((self.root / "result").exists())

    def test_runner_archives_topology_and_sets_workers_for_a_new_root(self):
        source = (ROOT / "run_trace_ablation.sh").read_text(encoding="utf-8")
        self.assertIn("EVAL_SCRATCH_ROOT:-/mnt/resources/nexus-evaluation/.scratch/e3", source)
        self.assertNotIn("data/traces/nexus-e3", source)
        self.assertNotIn("data/out/nexus-e3", source)
        self.assertIn('cluster_inventory_sha256=$(digest "$result_root/cluster-inventory.txt")', source)
        self.assertIn('remote_provenance_sha256=$(digest "$result_root/remote-provenance.txt")', source)
        self.assertIn('kubectl get nodes -o json | jq -S', source)
        self.assertIn('cp -- "$result_root/remote-provenance.txt" "$scratch_out/remote-provenance.txt"', source)
        self.assertIn('bin/nexus-backend', source)
        self.assertIn('bin/hardware-manager', source)
        self.assertIn('expected_invitro_head=$(git rev-parse HEAD)', source)
        self.assertIn('test "$head" = "$expected_head"', source)
        self.assertNotIn('remote_khala "$host" loader', source)
        self.assertNotIn('--vm-config "$vm_config"', source)
        self.assertIn('StrictHostKeyChecking=accept-new', source)
        self.assertNotIn('collect_e4_memory.py', source)
        self.assertNotIn('memory_sampler_sha256', source)
        self.assertNotIn('memory-sampler.log', source)
        self.assertNotIn('firecracker-memory.csv', source)
        self.assertNotIn('backend-memory.csv', source)
        self.assertNotIn('backend-memory-once.csv', source)

    def test_runner_rejects_scratch_inside_worktree(self):
        environment = os.environ.copy()
        environment["EVAL_SCRATCH_ROOT"] = str(ROOT / ".tmp" / "e3")
        result = subprocess.run(
            self.command("--dry-run"),
            cwd=ROOT,
            capture_output=True,
            text=True,
            env=environment,
        )
        self.assertEqual(result.returncode, 2)
        self.assertIn("must be outside the InVitro worktree", result.stderr)


if __name__ == "__main__":
    unittest.main()
