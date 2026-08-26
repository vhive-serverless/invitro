import unittest

from trace_modes import (
    MATCHED_WORKLOADS,
    MODE_INVM_PY,
    MODE_NEXUS_PY,
    MODE_NEXUS_GO,
    MODE_NEXUS_RDMA,
    MODE_NEXUS_RDMA_PY,
    MODE_HOSTTCP_GO,
    MODE_HOSTTCP_PY,
    TRACE_MODES,
    canonical_workload_name,
    trace_workload_name,
)


class TraceModeTest(unittest.TestCase):
    def test_mode_names_use_matched_workloads(self):
        expected = {
            MODE_INVM_PY: ("pyaesserve", "mapper", "reducer"),
            MODE_NEXUS_PY: (
                "pyaesserve-s3-rpc-shmem",
                "mapper-s3-rpc-shmem",
                "reducer-s3-rpc-shmem",
            ),
            MODE_NEXUS_GO: (
                "gopyaesserve-s3-rpc-shmem",
                "gomapper-s3-rpc-shmem",
                "goreducer-s3-rpc-shmem",
            ),
            MODE_NEXUS_RDMA: (
                "gopyaesserve-s3-rpc-rdma",
                "gomapper-s3-rpc-rdma",
                "goreducer-s3-rpc-rdma",
            ),
            MODE_NEXUS_RDMA_PY: (
                "pyaesserve-s3-rpc-rdma",
                "mapper-s3-rpc-rdma",
                "reducer-s3-rpc-rdma",
            ),
        }
        for mode, want in expected.items():
            self.assertEqual(
                tuple(trace_workload_name(name, mode) for name in MATCHED_WORKLOADS),
                want,
            )

    def test_transformed_names_keep_canonical_duration_identity(self):
        for canonical in MATCHED_WORKLOADS:
            for mode in (MODE_INVM_PY, MODE_NEXUS_PY, MODE_NEXUS_GO, MODE_NEXUS_RDMA, MODE_NEXUS_RDMA_PY, MODE_HOSTTCP_GO, MODE_HOSTTCP_PY):
                transformed = trace_workload_name(canonical, mode)
                recovered = canonical_workload_name(transformed)
                self.assertEqual(recovered, canonical)

    def test_hosttcp_modes_are_explicit_and_python_is_not_default(self):
        self.assertIn(MODE_HOSTTCP_GO, TRACE_MODES)
        self.assertIn(MODE_HOSTTCP_PY, TRACE_MODES)
        self.assertEqual(trace_workload_name("mapper", MODE_HOSTTCP_GO), "gomapper-s3-rpc-hosttcp")
        self.assertEqual(trace_workload_name("mapper", MODE_HOSTTCP_PY), "mapper-s3-rpc-hosttcp")

    def test_unmatched_workload_is_rejected(self):
        with self.assertRaises(ValueError):
            trace_workload_name("cnnserve", MODE_NEXUS_GO)


if __name__ == "__main__":
    unittest.main()
