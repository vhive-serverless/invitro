import csv
import json
import subprocess
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parent


class E2AdmissionTest(unittest.TestCase):
    def run_validator(self, items, expected=320):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            deployments = root / "deployments.json"
            evidence = root / "admission.csv"
            deployments.write_text(json.dumps({"items": items}), encoding="utf-8")
            result = subprocess.run(
                [
                    "python3",
                    str(ROOT / "experiment/e2/validate_admission.py"),
                    "--deployments",
                    str(deployments),
                    "--workload",
                    "mapper",
                    "--expected-replicas",
                    str(expected),
                    "--output",
                    str(evidence),
                ],
                text=True,
                capture_output=True,
            )
            rows = []
            if evidence.exists():
                with evidence.open(newline="", encoding="utf-8") as handle:
                    rows = list(csv.DictReader(handle))
            return result, rows

    @staticmethod
    def deployment(name, desired, ready):
        return {
            "metadata": {"name": name, "namespace": "default"},
            "spec": {"replicas": desired},
            "status": {
                "readyReplicas": ready,
                "availableReplicas": ready,
                "updatedReplicas": ready,
            },
        }

    def test_requires_exact_desired_and_ready_replicas(self):
        result, rows = self.run_validator([self.deployment("mapper-00001", 320, 320)])
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("admission_status=PASS", result.stdout)
        self.assertEqual(rows[-1]["function"], "__aggregate__")
        self.assertEqual(rows[-1]["ready_replicas"], "320")

        result, _ = self.run_validator([self.deployment("mapper-00001", 320, 319)])
        self.assertEqual(result.returncode, 1)
        self.assertIn("admission mismatch", result.stdout)

    def test_ignores_unrelated_deployments_and_rejects_no_match(self):
        result, _ = self.run_validator([self.deployment("reducer-00001", 320, 320)])
        self.assertEqual(result.returncode, 1)
        self.assertIn("no deployment matched workload mapper", result.stdout)


class E3EvidenceTest(unittest.TestCase):
    def run_validator(self, success_values):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            prefix = root / "experiment"
            with (root / "experiment_duration_1.csv").open(
                "w", newline="", encoding="utf-8"
            ) as handle:
                writer = csv.DictWriter(handle, fieldnames=["success"])
                writer.writeheader()
                for value in success_values:
                    writer.writerow({"success": str(value).lower()})
            return subprocess.run(
                [
                    "python3",
                    str(ROOT / "experiment/e3/validate_evidence.py"),
                    "--output-prefix",
                    str(prefix),
                ],
                text=True,
                capture_output=True,
            )

    def test_accepts_exactly_five_percent_failures(self):
        result = self.run_validator([True] * 19 + [False])
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("scientific_status=ACCEPTED", result.stdout)

    def test_rejects_more_than_five_percent_or_zero_successes(self):
        result = self.run_validator([True] * 18 + [False] * 2)
        self.assertEqual(result.returncode, 1)
        self.assertIn("exceeds 0.05", result.stdout)

        result = self.run_validator([False] * 20)
        self.assertEqual(result.returncode, 1)
        self.assertIn("zero successful invocations", result.stdout)


if __name__ == "__main__":
    unittest.main()
