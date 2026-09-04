#!/usr/bin/env python3
"""Fail-closed E2 evidence validation plus E2-Synth split counters."""
from __future__ import annotations

import argparse
import csv
import importlib.util
import json
import math
from pathlib import Path

EXPECTED_EVENTS = (
    "instructions:Hk", "cpu-cycles:Hk", "instructions:Gk", "cpu-cycles:Gk",
    "instructions:Hu", "cpu-cycles:Hu", "instructions:Gu", "cpu-cycles:Gu",
)


def load_base_validator():
    path = Path(__file__).parents[1] / "e2" / "validate_evidence.py"
    spec = importlib.util.spec_from_file_location("e2_base_validate_evidence", path)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"cannot load {path}")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def event_values(path: Path) -> dict[str, float]:
    rows: dict[str, list[float]] = {event: [] for event in EXPECTED_EVENTS}
    with path.open(newline="", encoding="utf-8") as handle:
        for row in csv.reader(handle):
            if not row:
                continue
            if len(row) >= 3 and row[2].strip() in rows:
                event, raw = row[2].strip(), row[0].strip()
            elif len(row) >= 2 and row[0].strip() in rows:
                event, raw = row[0].strip(), row[1].strip()
            else:
                continue
            try:
                value = float(raw)
            except ValueError as error:
                raise ValueError(f"{path}: nonnumeric {event} value {raw!r}") from error
            if not math.isfinite(value) or value < 0:
                raise ValueError(f"{path}: invalid {event} value {raw!r}")
            rows[event].append(value)
    missing = [event for event, values in rows.items() if not values]
    duplicate = [event for event, values in rows.items() if len(values) > 1]
    if missing or duplicate:
        raise ValueError(f"{path}: split-event contract mismatch; missing={missing} duplicate={duplicate}")
    return {event: values[0] for event, values in rows.items()}


def parse_bool(value: str) -> bool:
    if value == "true": return True
    if value == "false": return False
    raise argparse.ArgumentTypeError("expected true or false")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--output-prefix", required=True, type=Path)
    parser.add_argument("--loader-log", required=True, type=Path)
    parser.add_argument("--worker-config", required=True, type=Path)
    parser.add_argument("--perf-enabled", required=True, type=parse_bool)
    args = parser.parse_args()
    base = load_base_validator()
    try:
        with args.worker_config.open(encoding="utf-8") as handle:
            workers = json.load(handle).get("worker_nodes")
        if not isinstance(workers, list) or not workers or not all(isinstance(item, str) and item for item in workers):
            raise ValueError("worker config contains no valid worker_nodes")
        duration_count, cluster_count, kn_count, scale_count = base.validate_metrics(args.output_prefix, args.loader_log)
        artifacts = base.validate_perf(args.output_prefix, len(workers), args.perf_enabled)
        if args.perf_enabled:
            perf_csvs = [path for path in artifacts if path.suffix == ".csv"]
            if len(perf_csvs) != len(workers):
                raise ValueError("one perf CSV per worker is required")
            for path in perf_csvs:
                event_values(path)
    except (OSError, ValueError, json.JSONDecodeError) as error:
        print(f"evidence_status=FAIL reason={error}")
        return 1
    print("evidence_status=PASS")
    print(f"duration_sample_count={duration_count}")
    print(f"cluster_sample_count={cluster_count}")
    print(f"knative_sample_count={kn_count}")
    print(f"deployment_sample_count={scale_count}")
    print(f"perf_artifact_count={len(artifacts)}")
    print("required_split_events=" + ",".join(EXPECTED_EVENTS))
    for path in artifacts:
        print(f"perf_artifact={path.name}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
