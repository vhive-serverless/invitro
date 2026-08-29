#!/usr/bin/env python3
"""Fail-closed admission for one E2 loader output prefix."""

from __future__ import annotations

import argparse
import csv
import json
from pathlib import Path


DEPLOYMENT_ERROR_MARKERS = (
    "failed to deploy function",
)

TELEMETRY_ERROR_MARKERS = (
    "error querying prometheus",
    "fail to parse cluster usage",
    "fail to parse deployment scales",
    "fail to parse knative",
)


def one_nonempty(prefix: Path, suffix: str) -> Path:
    matches = sorted(prefix.parent.glob(prefix.name + suffix))
    if len(matches) != 1:
        raise ValueError(f"expected one {prefix.name + suffix}, found {len(matches)}")
    if matches[0].stat().st_size == 0:
        raise ValueError(f"empty evidence artifact: {matches[0]}")
    return matches[0]


def read_csv(path: Path) -> list[dict[str, str]]:
    with path.open(newline="", encoding="utf-8") as handle:
        rows = list(csv.DictReader(handle))
    if not rows:
        raise ValueError(f"no data rows in {path}")
    return rows


def validate_metrics(prefix: Path, loader_log: Path) -> tuple[int, int, int, int]:
    log_text = loader_log.read_text(encoding="utf-8", errors="replace").lower()
    deployment_failures = [marker for marker in DEPLOYMENT_ERROR_MARKERS if marker in log_text]
    if deployment_failures:
        raise ValueError("loader recorded function deployment failure: " + ", ".join(deployment_failures))
    found = [marker for marker in TELEMETRY_ERROR_MARKERS if marker in log_text]
    if found:
        raise ValueError("loader recorded telemetry transport/parser errors: " + ", ".join(found))

    duration_rows = read_csv(one_nonempty(prefix, "_duration_*.csv"))
    if not any(row.get("success", "").lower() == "true" for row in duration_rows):
        raise ValueError("duration evidence contains no successful invocation")

    cluster_path = one_nonempty(prefix, "_cluster_usage_*.csv")
    cluster_rows = []
    with cluster_path.open(encoding="utf-8") as handle:
        for line_number, line in enumerate(handle, 1):
            if line.strip():
                try:
                    cluster_rows.append(json.loads(line))
                except json.JSONDecodeError as error:
                    raise ValueError(f"invalid cluster JSON at {cluster_path}:{line_number}: {error}") from error
    if not cluster_rows:
        raise ValueError(f"no cluster samples in {cluster_path}")
    if not any(isinstance(row.get("hardware_metrics"), dict) and row["hardware_metrics"] for row in cluster_rows):
        raise ValueError("cluster telemetry contains no hardware-manager sample")
    if not any(isinstance(row.get("cpu"), list) and any(str(value).strip() for value in row["cpu"]) for row in cluster_rows):
        raise ValueError("cluster telemetry contains no node CPU sample")

    kn_rows = read_csv(one_nonempty(prefix, "_kn_stats_*.csv"))
    core_fields = ("desired_pods", "unready_pods", "pending_pods", "requested_pods", "running_pods")
    if not any(all(row.get(field, "").lstrip("-").isdigit() and int(row[field]) >= 0 for field in core_fields) for row in kn_rows):
        raise ValueError("Knative telemetry is sentinel-only")

    scale_rows = read_csv(one_nonempty(prefix, "_deployment_scale_*.csv"))
    if not any(row.get("function") and row.get("running_pods", "").isdigit() and int(row["running_pods"]) > 0 for row in scale_rows):
        raise ValueError("deployment telemetry contains no running function sample")
    return len(duration_rows), len(cluster_rows), len(kn_rows), len(scale_rows)


def validate_perf(prefix: Path, worker_count: int, enabled: bool) -> list[Path]:
    if not enabled:
        return []
    artifacts: list[Path] = []
    for worker_index in range(worker_count):
        paths = [
            Path(f"{prefix}_perf_{worker_index}.csv"),
            Path(f"{prefix}_perf_{worker_index}.data"),
            Path(f"{prefix}_perf_{worker_index}.svg"),
            Path(f"{prefix}_perf_filtered_{worker_index}.svg"),
        ]
        for path in paths:
            if not path.is_file() or path.stat().st_size == 0:
                raise ValueError(f"missing or empty perf artifact: {path}")
        for path in (paths[2], paths[3]):
            if "<svg" not in path.read_text(encoding="utf-8", errors="replace"):
                raise ValueError(f"invalid flame graph artifact: {path}")
        artifacts.extend(paths)
    return artifacts


def parse_bool(value: str) -> bool:
    if value == "true":
        return True
    if value == "false":
        return False
    raise argparse.ArgumentTypeError("expected true or false")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--output-prefix", required=True, type=Path)
    parser.add_argument("--loader-log", required=True, type=Path)
    parser.add_argument("--worker-config", required=True, type=Path)
    parser.add_argument("--perf-enabled", required=True, type=parse_bool)
    args = parser.parse_args()

    with args.worker_config.open(encoding="utf-8") as handle:
        worker_config = json.load(handle)
    workers = worker_config.get("worker_nodes")
    if not isinstance(workers, list) or not workers or not all(isinstance(item, str) and item for item in workers):
        raise SystemExit("worker config contains no valid worker_nodes")

    try:
        duration_count, cluster_count, kn_count, scale_count = validate_metrics(args.output_prefix, args.loader_log)
        perf = validate_perf(args.output_prefix, len(workers), args.perf_enabled)
    except (OSError, ValueError, json.JSONDecodeError) as error:
        print(f"evidence_status=FAIL reason={error}")
        return 1

    print("evidence_status=PASS")
    print(f"duration_sample_count={duration_count}")
    print(f"cluster_sample_count={cluster_count}")
    print(f"knative_sample_count={kn_count}")
    print(f"deployment_sample_count={scale_count}")
    print(f"perf_artifact_count={len(perf)}")
    for path in perf:
        print(f"perf_artifact={path.name}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
