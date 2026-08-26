import unittest

from collect_e4_memory import summarize


class E4MemoryTest(unittest.TestCase):
    def test_summarize_separates_firecracker_backend_and_worker_memory(self):
        payload = {
            "processes": [
                {"name": "firecracker", "pid": 1, "pss_kib": 100},
                {"name": "firecracker", "pid": 2, "pss_kib": 140},
                {"name": "nexus-backend", "pid": 3, "pss_kib": 80},
            ],
            "memtotal_kib": 1000,
            "memavailable_kib": 400,
        }
        sample, backend = summarize(payload, "10.0.1.3", "nexus-py", 2, "now")
        self.assertEqual(sample["firecracker_processes"], 2)
        self.assertEqual(sample["firecracker_total_pss_kib"], 240)
        self.assertEqual(sample["firecracker_mean_pss_kib"], 120)
        self.assertEqual(sample["worker_used_kib"], 600)
        self.assertEqual(backend["backend_processes"], 1)
        self.assertEqual(backend["backend_total_pss_kib"], 80)

    def test_invalid_worker_memory_is_rejected(self):
        with self.assertRaises(ValueError):
            summarize({"processes": [], "memtotal_kib": 10, "memavailable_kib": 20},
                      "worker", "invm-py", 0, "now")


if __name__ == "__main__":
    unittest.main()
