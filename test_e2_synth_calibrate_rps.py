#!/usr/bin/env python3
import csv
import tempfile
import unittest
from pathlib import Path

from e2_synth_calibrate_rps import (
    PAYLOADS, STEPS, analyze, build_plan, extract_e1_unloaded, merge_references,
    observe_duration, parse_payloads, read_averages, resolve_refinements, write_rows,
)


class E2SynthCalibrationTests(unittest.TestCase):
    def setUp(self):
        self.averages = {payload: 1.0 for payload in PAYLOADS}
        self.plan = build_plan(self.averages, 28)

    def observations(self, payload, fail_step=None):
        rows = []
        for step, rps in enumerate(self.plan[payload]["levels"], 1):
            failed = fail_step is not None and step >= fail_step
            rows.append({
                "payload_bytes": str(payload), "step": str(step), "rps": str(rps),
                "issued": "100", "successful": "90" if failed else "100",
                "failed": "10" if failed else "0",
                "failure_rate": "0.1" if failed else "0",
                "p99_ms": "1",
            })
        return rows

    def test_boundary_right_censored_and_first_level_failure(self):
        payload = PAYLOADS[0]
        boundary = analyze(self.plan, self.observations(payload, 8), [payload], "current")
        self.assertEqual(boundary[0]["status"], "BOUNDARY_OBSERVED")
        right = analyze(self.plan, self.observations(payload), [payload], "current")
        self.assertEqual(right[0]["status"], "RIGHT_CENSORED")
        first = analyze(self.plan, self.observations(payload, 1), [payload], "current")
        self.assertEqual(first[0]["status"], "NO_ADMISSIBLE_LEVEL")

    def test_missing_minute_rejected(self):
        payload = PAYLOADS[0]
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "duration.csv"
            with path.open("w", newline="", encoding="utf-8") as handle:
                writer = csv.DictWriter(handle, fieldnames=("phase", "invocationID", "responseTime", "success"))
                writer.writeheader()
                for minute in range(1, STEPS):
                    writer.writerow({"phase": 2, "invocationID": f"min{minute}.inv0", "responseTime": 1000, "success": "true"})
            with self.assertRaisesRegex(ValueError, "expected 20 measured minutes"):
                observe_duration(path, payload, self.plan[payload])

    def test_duplicate_payload_summary_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "averages.csv"
            path.write_text("payload_bytes,unloaded_average_ms,n_samples\n4,1,2\n4,1,2\n", encoding="utf-8")
            with self.assertRaisesRegex(ValueError, "duplicate"):
                read_averages(path, require_complete=False)

    def test_both_authorized_halves(self):
        current = parse_payloads("4,16384,262144,2097152,8388608")
        supplied = parse_payloads("4096,65536,1048576,4194304,16777216")
        self.assertEqual(set(current) | set(supplied), set(PAYLOADS))
        self.assertFalse(set(current) & set(supplied))
        self.assertEqual(len(build_plan(self.averages, 28, selected=current)), 5)
        self.assertEqual(len(build_plan(self.averages, 28, selected=supplied)), 5)

    def test_extracts_arithmetic_warm_mean(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            for payload in PAYLOADS:
                path = root / f"invm-py_synthetic_e_0_p_{payload}_latency" / "event.csv"
                path.parent.mkdir()
                path.write_text("event\n\"warm_e2e_ns: 1000000\"\n\"warm_e2e_ns: 3000000\"\n", encoding="utf-8")
            rows = extract_e1_unloaded(root)
            self.assertEqual(rows[0]["unloaded_average_ms"], 2.0)
            self.assertEqual(rows[0]["n_samples"], 2)

    def test_merge_requires_disjoint_complete_halves(self):
        fields = ("payload_bytes", "calibration_cluster", "unloaded_average_ms", "worker_cores",
                  "ceiling_multiplier", "rbound", "first_failing_step", "first_failing_rps",
                  "rmax_b0", "rref", "status", "reference_kind")
        with tempfile.TemporaryDirectory() as directory:
            paths = [Path(directory) / "a.csv", Path(directory) / "b.csv"]
            halves = (PAYLOADS[::2], PAYLOADS[1::2])
            for path, half, cluster in zip(paths, halves, ("current", "supplied")):
                rows = [{key: ({"payload_bytes": payload, "calibration_cluster": cluster,
                                "unloaded_average_ms": 1, "worker_cores": 28,
                                "ceiling_multiplier": 1, "rbound": 100, "rmax_b0": 50,
                                "rref": 25, "status": "BOUNDARY_OBSERVED",
                                "reference_kind": "OBSERVED_BOUNDARY_REFERENCE"}.get(key, ""))
                         for key in fields} for payload in half]
                write_rows(path, rows)
            self.assertEqual(len(merge_references(paths)), 10)
            with paths[1].open("r+", encoding="utf-8") as handle:
                text = handle.read().replace(str(PAYLOADS[1]), str(PAYLOADS[0]), 1)
                handle.seek(0); handle.write(text); handle.truncate()
            with self.assertRaisesRegex(ValueError, "duplicate"):
                merge_references(paths)

    def test_resolve_refinement_replaces_only_unresolved_row(self):
        fields = ("payload_bytes", "calibration_cluster", "unloaded_average_ms", "worker_cores",
                  "ceiling_multiplier", "rbound", "first_failing_step", "first_failing_rps",
                  "rmax_b0", "rref", "status", "reference_kind")
        with tempfile.TemporaryDirectory() as directory:
            base_path = Path(directory) / "base.csv"
            refinement_path = Path(directory) / "refinement.csv"
            rows = []
            for payload in PAYLOADS[::2]:
                values = {"payload_bytes": payload, "calibration_cluster": "current",
                          "unloaded_average_ms": 1, "worker_cores": 28,
                          "ceiling_multiplier": 1, "rbound": 100, "rmax_b0": 50,
                          "rref": 25, "status": "BOUNDARY_OBSERVED",
                          "reference_kind": "OBSERVED_BOUNDARY_REFERENCE"}
                rows.append({key: values.get(key, "") for key in fields})
            unresolved = PAYLOADS[::2][-1]
            rows[-1].update({"rmax_b0": "", "rref": "", "status": "NO_ADMISSIBLE_LEVEL",
                             "reference_kind": "NO_REFERENCE"})
            write_rows(base_path, rows)
            refined = dict(rows[-1])
            refined.update({"ceiling_multiplier": 0.1, "rbound": 20, "rmax_b0": 10,
                            "rref": 5, "status": "BOUNDARY_OBSERVED",
                            "reference_kind": "OBSERVED_BOUNDARY_REFERENCE"})
            write_rows(refinement_path, [refined])
            resolved = resolve_refinements(base_path, [refinement_path])
            self.assertEqual(len(resolved), 5)
            self.assertEqual(resolved[-1]["rref"], "5")
            self.assertEqual(resolved[-1]["ceiling_multiplier"], "0.1")
            with self.assertRaisesRegex(ValueError, "not unresolved"):
                resolve_refinements(base_path, [refinement_path, refinement_path])


if __name__ == "__main__":
    unittest.main()
