import json
import os
import stat
import subprocess
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parent
GATE = ROOT / "scripts" / "util" / "wait_prometheus_ready.sh"
CLEAN = ROOT / "scripts" / "util" / "clean_prometheus.sh"


class PrometheusReadinessTest(unittest.TestCase):
    def make_fake_tools(self, ready_after=2, fail_query=False):
        temp = tempfile.TemporaryDirectory()
        bindir = Path(temp.name)
        state = bindir / "calls"
        state.write_text("0", encoding="utf-8")
        kubectl = bindir / "kubectl"
        kubectl.write_text(
            "#!/usr/bin/env bash\n"
            "set -euo pipefail\n"
            f"state={state!s}\n"
            "case \"$*\" in\n"
            "  *'rollout status'*) exit 0 ;;\n"
            "  *'get service'*) printf '10.96.0.10\\n' ;;\n"
            "  *'get endpoints'*) printf '10.0.0.1\\n' ;;\n"
            "  *'rollout restart'*) exit 0 ;;\n"
            "  *'get statefulset'*) exit 0 ;;\n"
            "  *) exit 1 ;;\n"
            "esac\n",
            encoding="utf-8",
        )
        curl = bindir / "curl"
        query_body = "  exit 22\n" if fail_query else (
            "  printf '" + json.dumps({"status": "success", "data": {"result": [{"metric": {"job": "node-exporter"}, "value": ["0", "1"]}]}}) + "\\n'\n"
        )
        curl.write_text(
            "#!/usr/bin/env bash\n"
            "set -euo pipefail\n"
            f"state={state!s}\n"
            "n=$(cat \"$state\")\n"
            "if [[ \"$*\" == *'/-/ready'* ]]; then\n"
            f"  if ((n < {ready_after})); then echo $((n+1)) > \"$state\"; exit 22; fi\n"
            "  printf 'Prometheus Ready\\n'; exit 0\n"
            "fi\n"
            + query_body,
            encoding="utf-8",
        )
        for tool in (kubectl, curl):
            tool.chmod(tool.stat().st_mode | stat.S_IXUSR)
        return temp, bindir, state

    def test_delayed_readiness_is_retried(self):
        temp, bindir, state = self.make_fake_tools(ready_after=2)
        self.addCleanup(temp.cleanup)
        env = os.environ.copy()
        env.update({"PATH": f"{bindir}:{env['PATH']}", "PROMETHEUS_READY_TIMEOUT_SECONDS": "5", "PROMETHEUS_READY_POLL_SECONDS": "1"})
        result = subprocess.run([str(GATE)], env=env, capture_output=True, text=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(state.read_text(encoding="utf-8").strip(), "2")

    def test_query_failure_propagates(self):
        temp, bindir, _ = self.make_fake_tools(ready_after=0, fail_query=True)
        self.addCleanup(temp.cleanup)
        env = os.environ.copy()
        env.update({"PATH": f"{bindir}:{env['PATH']}", "PROMETHEUS_READY_TIMEOUT_SECONDS": "1", "PROMETHEUS_READY_POLL_SECONDS": "1"})
        result = subprocess.run([str(GATE)], env=env, capture_output=True, text=True)
        self.assertEqual(result.returncode, 1)
        self.assertIn("up query", result.stderr)


if __name__ == "__main__":
    unittest.main()
