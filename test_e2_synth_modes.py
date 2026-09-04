import unittest

from e2_synth_modes import MODES, PAYLOADS, attaches_shared_memory, canonical_mode, workload_name


class ModeContractTests(unittest.TestCase):
    def test_canonical_names_and_legacy_alias(self):
        self.assertEqual(canonical_mode("nexus-rdma"), "nexus-rdma-go")
        self.assertEqual(len(MODES), 9)
        self.assertEqual(len(PAYLOADS), 10)

    def test_trace_names_and_attachment_policy(self):
        payload = 16777216
        wants = {
            "invm-py": f"synthetic_e_0_p_{payload}",
            "invm-js": f"jssynthetic_e_0_p_{payload}",
            "invm-go": f"gosynthetic_e_0_p_{payload}",
            "hosttcp-go": f"gosynthetic_e_0_p_{payload}-s3-rpc-hosttcp",
            "nexus-py": f"synthetic_e_0_p_{payload}-s3-rpc-shmem",
            "nexus-js": f"jssynthetic_e_0_p_{payload}-s3-rpc-shmem",
            "nexus-go": f"gosynthetic_e_0_p_{payload}-s3-rpc-shmem",
            "nexus-rdma-py": f"synthetic_e_0_p_{payload}-s3-rpc-rdma",
            "nexus-rdma-go": f"gosynthetic_e_0_p_{payload}-s3-rpc-rdma",
        }
        for mode, want in wants.items():
            self.assertEqual(workload_name(payload, mode), want)
            self.assertEqual(attaches_shared_memory(mode), not mode.startswith("invm-"))

    def test_noncanonical_payload_rejected(self):
        with self.assertRaises(ValueError):
            workload_name(5, "invm-py")


if __name__ == "__main__":
    unittest.main()
