import subprocess
import unittest


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
        for mode in ("invm-py", "nexus-go", "nexus-rdma"):
            self.assertIn(f"=== mode={mode} ", output)
            self.assertIn(f"--mode {mode}", output)
        self.assertIn("gopyaesserve-s3-rpc-stream-0", output)
        self.assertIn("gopyaesserve-s3-rpc-0", output)
        self.assertNotIn("nexusprefetch", output)
        self.assertNotIn("sdkonly", output)

        for line in output.splitlines():
            if line.startswith("CLEAN:"):
                self.assertNotIn("--stream-slots", line)
                self.assertNotIn("--stream-capacity", line)


if __name__ == "__main__":
    unittest.main()
