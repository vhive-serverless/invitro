import csv
import tempfile
import unittest
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))
from validate_evidence import EXPECTED_EVENTS, event_values


class SplitEventTests(unittest.TestCase):
    def fixture(self, mutate=None):
        directory = tempfile.TemporaryDirectory()
        path = Path(directory.name) / "perf.csv"
        rows = [["1", "", event] for event in EXPECTED_EVENTS]
        if mutate: mutate(rows)
        with path.open("w", newline="", encoding="utf-8") as handle:
            csv.writer(handle).writerows(rows)
        return directory, path

    def test_accepts_exact_numeric_events(self):
        directory, path = self.fixture()
        self.addCleanup(directory.cleanup)
        self.assertEqual(set(event_values(path)), set(EXPECTED_EVENTS))

    def test_rejects_missing(self):
        directory, path = self.fixture(lambda rows: rows.pop())
        self.addCleanup(directory.cleanup)
        with self.assertRaisesRegex(ValueError, "missing"):
            event_values(path)

    def test_rejects_duplicate(self):
        directory, path = self.fixture(lambda rows: rows.append(rows[0]))
        self.addCleanup(directory.cleanup)
        with self.assertRaisesRegex(ValueError, "duplicate"):
            event_values(path)

    def test_rejects_not_counted(self):
        def mutate(rows): rows[0][0] = "<not counted>"
        directory, path = self.fixture(mutate)
        self.addCleanup(directory.cleanup)
        with self.assertRaisesRegex(ValueError, "nonnumeric"):
            event_values(path)


if __name__ == "__main__":
    unittest.main()
