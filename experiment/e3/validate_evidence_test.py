import csv
import tempfile
import unittest
from pathlib import Path

from experiment.e3 import validate_evidence


class EvidenceSummaryTest(unittest.TestCase):
    def test_threshold_failure_keeps_exact_counts(self):
        with tempfile.TemporaryDirectory() as directory:
            prefix = Path(directory) / "experiment"
            path = Path(directory) / "experiment_duration_3.csv"
            with path.open("w", newline="", encoding="utf-8") as handle:
                writer = csv.DictWriter(handle, fieldnames=["success"])
                writer.writeheader()
                for success in [True] * 94 + [False] * 6:
                    writer.writerow({"success": success})
            successes, failures, fraction = validate_evidence.summarize(prefix)
            self.assertEqual((successes, failures), (94, 6))
            self.assertEqual(str(fraction), "0.06")
            with self.assertRaisesRegex(ValueError, "exceeds 0.05"):
                validate_evidence.validate(prefix)


if __name__ == "__main__":
    unittest.main()
