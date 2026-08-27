import csv
import tempfile
import unittest
from pathlib import Path

from generate_trace_sweep import Config, SweepTraceBuilder, read_e2_reference

from trace_modes import (
    DENSITY_WORKLOADS,
    MODE_HOSTTCP_GO,
    MODE_HOSTTCP_PY,
    MODE_INVM_GO,
    MODE_INVM_JS,
    MODE_INVM_PY,
    MODE_NEXUS_GO,
    MODE_NEXUS_JS,
    MODE_NEXUS_PY,
    MODE_NEXUS_RDMA,
    MODE_NEXUS_RDMA_PY,
    PYTHON_WORKLOADS,
    TRACE_MODES,
    canonical_workload_name,
    trace_workload_name,
)


class TraceModeTest(unittest.TestCase):
    def test_full_python_workload_names_cover_e1(self):
        for workload in PYTHON_WORKLOADS:
            self.assertEqual(trace_workload_name(workload, MODE_INVM_PY), workload)
            self.assertEqual(trace_workload_name(workload, MODE_NEXUS_PY), f"{workload}-s3-rpc-shmem")
            self.assertEqual(trace_workload_name(workload, MODE_NEXUS_RDMA_PY), f"{workload}-s3-rpc-rdma")

    def test_density_set_excludes_only_helloworld(self):
        self.assertEqual(len(DENSITY_WORKLOADS), 10)
        self.assertNotIn("helloworld", DENSITY_WORKLOADS)
        self.assertEqual(set(DENSITY_WORKLOADS), set(PYTHON_WORKLOADS) - {"helloworld"})

    def test_hello_language_and_transport_names(self):
        expected = {
            MODE_INVM_GO: "gohelloworld",
            MODE_INVM_JS: "jshelloworld",
            MODE_HOSTTCP_GO: "gohelloworld-s3-rpc-hosttcp",
            MODE_NEXUS_GO: "gohelloworld-s3-rpc-shmem",
            MODE_NEXUS_JS: "jshelloworld-s3-rpc-shmem",
            MODE_NEXUS_RDMA: "gohelloworld-s3-rpc-rdma",
        }
        for mode, want in expected.items():
            self.assertEqual(trace_workload_name("helloworld", mode), want)
            self.assertEqual(canonical_workload_name(want), "helloworld")

    def test_matched_go_ablation_names_remain_available(self):
        expected = {
            "pyaesserve": "gopyaesserve",
            "mapper": "gomapper",
            "reducer": "goreducer",
        }
        for workload, prefix in expected.items():
            self.assertEqual(trace_workload_name(workload, MODE_INVM_GO), prefix)
            self.assertEqual(trace_workload_name(workload, MODE_NEXUS_GO), f"{prefix}-s3-rpc-shmem")
            self.assertEqual(trace_workload_name(workload, MODE_HOSTTCP_GO), f"{prefix}-s3-rpc-hosttcp")

    def test_unimplemented_go_and_js_workloads_are_rejected(self):
        with self.assertRaises(ValueError):
            trace_workload_name("cnnserve", MODE_NEXUS_GO)
        with self.assertRaises(ValueError):
            trace_workload_name("mapper", MODE_NEXUS_JS)

    def test_hosttcp_python_is_parseable_but_not_in_claim_matrix(self):
        self.assertIn(MODE_HOSTTCP_GO, TRACE_MODES)
        self.assertIn(MODE_HOSTTCP_PY, TRACE_MODES)
        self.assertEqual(trace_workload_name("mapper", MODE_HOSTTCP_PY), "mapper-s3-rpc-hosttcp")

    def test_e2_reference_supplies_all_density_rps_and_durations(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "reference.csv"
            with path.open("w", newline="", encoding="utf-8") as handle:
                writer = csv.DictWriter(handle, fieldnames=["workload", "unloaded_average_ms", "rref", "status"])
                writer.writeheader()
                writer.writerow({"workload": "helloworld", "unloaded_average_ms": 1, "rref": 1, "status": "BOUNDARY_OBSERVED"})
                for index, workload in enumerate(DENSITY_WORKLOADS, 1):
                    writer.writerow({"workload": workload, "unloaded_average_ms": index, "rref": 100 + index, "status": "BOUNDARY_OBSERVED"})
            rps, durations = read_e2_reference(path)
            self.assertEqual(set(rps), set(DENSITY_WORKLOADS))
            self.assertEqual(set(durations), set(DENSITY_WORKLOADS))
            self.assertEqual(rps[DENSITY_WORKLOADS[0]], 101)

    def test_e2_reference_accepts_labeled_right_censored_reference(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "reference.csv"
            with path.open("w", newline="", encoding="utf-8") as handle:
                writer = csv.DictWriter(handle, fieldnames=["workload", "unloaded_average_ms", "rref", "status", "reference_kind", "rmax_b0"])
                writer.writeheader()
                for workload in DENSITY_WORKLOADS:
                    writer.writerow({"workload": workload, "unloaded_average_ms": 10, "rref": 10,
                                     "status": "RIGHT_CENSORED" if workload == "rnnserve" else "BOUNDARY_OBSERVED",
                                     "reference_kind": "RIGHT_CENSORED_REFERENCE" if workload == "rnnserve" else "OBSERVED_BOUNDARY_REFERENCE",
                                     "rmax_b0": "" if workload == "rnnserve" else "20"})
            rps, _ = read_e2_reference(path)
            self.assertEqual(rps["rnnserve"], 10)

    def test_e2_reference_rejects_unlabeled_right_censoring(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "reference.csv"
            with path.open("w", newline="", encoding="utf-8") as handle:
                writer = csv.DictWriter(handle, fieldnames=["workload", "unloaded_average_ms", "rref", "status", "reference_kind", "rmax_b0"])
                writer.writeheader()
                for workload in DENSITY_WORKLOADS:
                    writer.writerow({"workload": workload, "unloaded_average_ms": 10, "rref": 10,
                                     "status": "RIGHT_CENSORED" if workload == "rnnserve" else "BOUNDARY_OBSERVED",
                                     "reference_kind": "", "rmax_b0": ""})
            with self.assertRaisesRegex(ValueError, "rnnserve"):
                read_e2_reference(path)

    def test_standard_library_generator_preserves_staggered_activation(self):
        with tempfile.TemporaryDirectory() as directory:
            input_path = Path(directory) / "invocations.csv"
            with input_path.open("w", newline="", encoding="utf-8") as handle:
                writer = csv.DictWriter(handle, fieldnames=["HashFunction", "Trigger", "1", "2", "3", "4"])
                writer.writeheader()
                writer.writerow({"HashFunction": "f", "Trigger": "http", "1": 50, "2": 50, "3": 50, "4": 50})
            builder = SweepTraceBuilder(Config(
                input_path=input_path,
                output_dir=Path(directory) / "out",
                workload_rps={"mapper": 1},
                workload_duration_ms={"mapper": 10},
                divisor=1,
                start_scale=1,
                end_scale=3,
                step=1,
                warmup_duration=2,
            ))
            invocations, durations = builder.run()
            self.assertEqual(len(invocations), 3)
            self.assertGreater(int(invocations[0]["-1"]), 0)
            self.assertEqual(int(invocations[1]["-1"]), 0)
            self.assertEqual(int(invocations[1]["1"]), 0)
            self.assertEqual(int(invocations[1]["2"]), 50)
            self.assertEqual(int(invocations[2]["2"]), 0)
            self.assertEqual(int(invocations[2]["3"]), 50)
            self.assertEqual(len(durations), 3)


if __name__ == "__main__":
    unittest.main()
