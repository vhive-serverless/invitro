import unittest

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


if __name__ == "__main__":
    unittest.main()
