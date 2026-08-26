import subprocess
import unittest
import os
from pathlib import Path


ROOT = Path(__file__).resolve().parent


class AblationDryRunTest(unittest.TestCase):
    def test_dry_run_is_explicit_and_side_effect_free(self):
        result = subprocess.run(
            ["bash", "run_trace_ablation.sh", "--dry-run"],
            check=True,
            capture_output=True,
            text=True,
        )
        output = result.stdout
        self.assertEqual(output.count("=== mode="), 3)
        for mode in ("invm-py", "nexus-py", "nexus-rdma-py"):
            self.assertIn(f"=== mode={mode} ", output)
            self.assertIn(f"--mode {mode}", output)
        self.assertIn("pyaesserve-s3-rpc-shmem-0", output)
        self.assertIn("pyaesserve-s3-rpc-rdma-0", output)
        self.assertNotIn("=== mode=nexus-go ", output)
        self.assertNotIn("hosttcp-go", output)
        self.assertNotIn("nexusprefetch", output)
        self.assertNotIn("sdkonly", output)

        for line in output.splitlines():
            if line.startswith("CLEAN:"):
                self.assertNotIn("--stream-slots", line)
                self.assertNotIn("--stream-capacity", line)
            if line.startswith("DEPLOY:"):
                self.assertNotIn("--tcp-transport", line)
                self.assertIn("--shmem-ring-bytes 4190208", line)
                self.assertIn("--minio-endpoint 10.0.1.4:9001", line)

    def test_hosttcp_is_opt_in_via_mode_list(self):
        result = subprocess.run(
            ["bash", "run_trace_ablation.sh", "--dry-run"],
            cwd=ROOT,
            env={**os.environ, "MODE_LIST": "invm-py,hosttcp-go"},
            capture_output=True,
            text=True,
            check=True,
        )
        self.assertIn("=== mode=hosttcp-go ", result.stdout)
        self.assertIn("guest-owned=sdk,signing,http,tls,grpc,protobuf", result.stdout)
        self.assertIn("host-owned=dns,tcp,opaque-relay", result.stdout)
        self.assertIn("--mode hosttcp-go", result.stdout)
        self.assertIn("--shmem-ring-bytes 4190208", result.stdout)
        self.assertIn("--shmem-io-quantum 262144", result.stdout)
        self.assertIn("CONFIG: EXPERIMENT=hosttcp-go_d-100_s-1_e-27_t-1", result.stdout)
        self.assertIn("LOAD: go run cmd/loader.go --config cmd/config_khala_trace.json", result.stdout)
        self.assertNotIn("=== mode=nexus-go ", result.stdout)

    def test_unknown_mode_fails_before_printing_or_side_effects(self):
        result = subprocess.run(
            ["bash", "run_trace_ablation.sh", "--dry-run"],
            cwd=ROOT,
            env={**os.environ, "MODE_LIST": "invm-py,unknown-mode"},
            capture_output=True,
            text=True,
        )
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("unsupported mode: unknown-mode", result.stderr)
        self.assertNotIn("=== mode=", result.stdout)


if __name__ == "__main__":
    unittest.main()
