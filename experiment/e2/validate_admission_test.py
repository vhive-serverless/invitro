import unittest

from experiment.e2 import validate_admission


def deployment(name: str, replicas: int = 320) -> dict:
    return {
        "metadata": {
            "name": name + "-0-00001-deployment",
            "namespace": "default",
            "labels": {"serving.knative.dev/service": name + "-0"},
        },
        "spec": {"replicas": replicas},
        "status": {
            "readyReplicas": replicas,
            "availableReplicas": replicas,
            "updatedReplicas": replicas,
        },
    }


class AdmissionIsolationTest(unittest.TestCase):
    def test_accepts_exactly_one_admitted_deployment(self):
        rows = validate_admission.validate(
            {"items": [deployment("gohelloworld-s3-rpc-hosttcp")]},
            "gohelloworld-s3-rpc-hosttcp",
            320,
        )
        self.assertEqual(len(rows), 1)
        self.assertEqual(rows[0]["ready_replicas"], 320)

    def test_rejects_a_second_live_deployment(self):
        with self.assertRaisesRegex(ValueError, "outside the admitted function"):
            validate_admission.validate(
                {
                    "items": [
                        deployment("gohelloworld-s3-rpc-hosttcp"),
                        deployment("previous-cell"),
                    ]
                },
                "gohelloworld-s3-rpc-hosttcp",
                320,
            )

    def test_rejects_two_matching_revisions(self):
        first = deployment("gohelloworld-s3-rpc-hosttcp")
        second = deployment("gohelloworld-s3-rpc-hosttcp")
        second["metadata"]["name"] = "gohelloworld-s3-rpc-hosttcp-0-00002-deployment"
        with self.assertRaisesRegex(ValueError, "exactly one deployment"):
            validate_admission.validate(
                {"items": [first, second]}, "gohelloworld-s3-rpc-hosttcp", 320
            )


if __name__ == "__main__":
    unittest.main()
