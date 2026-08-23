import unittest

from trace_modes import (
    MATCHED_WORKLOADS,
    MODE_INVM_PY,
    MODE_NEXUS_GO,
    MODE_NEXUS_RDMA,
    canonical_workload_name,
    trace_workload_name,
)


class TraceModeTest(unittest.TestCase):
    def test_mode_names_use_matched_workloads(self):
        expected = {
            MODE_INVM_PY: ("pyaesserve", "mapper", "reducer"),
            MODE_NEXUS_GO: (
                "gopyaesserve-s3-rpc-stream",
                "gomapper-s3-rpc-stream",
                "goreducer-s3-rpc-stream",
            ),
            MODE_NEXUS_RDMA: (
                "gopyaesserve-s3-rpc",
                "gomapper-s3-rpc",
                "goreducer-s3-rpc",
            ),
        }
        for mode, want in expected.items():
            self.assertEqual(
                tuple(trace_workload_name(name, mode) for name in MATCHED_WORKLOADS),
                want,
            )

    def test_transformed_names_keep_canonical_duration_identity(self):
        for canonical in MATCHED_WORKLOADS:
            for mode in (MODE_INVM_PY, MODE_NEXUS_GO, MODE_NEXUS_RDMA):
                transformed = trace_workload_name(canonical, mode)
                recovered = canonical_workload_name(transformed)
                self.assertEqual(recovered, canonical)

    def test_unmatched_workload_is_rejected(self):
        with self.assertRaises(ValueError):
            trace_workload_name("cnnserve", MODE_NEXUS_GO)


if __name__ == "__main__":
    unittest.main()
