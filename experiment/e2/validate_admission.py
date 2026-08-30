#!/usr/bin/env python3
"""Validate and serialize the fixed-replica admission observed by Kubernetes.

The runner deliberately uses this small JSON contract instead of inferring
admission from the requested FixedReplicaCount in the loader config.
"""

from __future__ import annotations

import argparse
import csv
import hashlib
import json
from pathlib import Path


def _number(value: object, field: str) -> int:
    if isinstance(value, bool) or not isinstance(value, int) or value < 0:
        raise ValueError(f"{field} is not a non-negative integer")
    return value


def _matches(item: dict, workload: str) -> bool:
    metadata = item.get("metadata") or {}
    labels = metadata.get("labels") or {}
    identities = [metadata.get("name", "")]
    identities.extend(
        labels.get(key, "")
        for key in (
            "serving.knative.dev/service",
            "app",
            "function",
            "functionName",
        )
    )
    # Knative revision/deployment names append transport and revision
    # suffixes to the canonical workload name.
    return any(value == workload or value.startswith(workload + "-") for value in identities if value)


def validate(payload: dict, workload: str, expected: int) -> list[dict[str, object]]:
    items = payload.get("items")
    if not isinstance(items, list):
        raise ValueError("Kubernetes deployment response contains no items list")
    unexpected = []
    for item in items:
        if not isinstance(item, dict):
            unexpected.append("<malformed>")
        elif not _matches(item, workload):
            unexpected.append((item.get("metadata") or {}).get("name", "<unnamed>"))
    if unexpected:
        raise ValueError(
            "namespace contains deployment(s) outside the admitted function: "
            + ", ".join(str(name) for name in unexpected)
        )
    rows: list[dict[str, object]] = []
    for item in items:
        if not isinstance(item, dict) or not _matches(item, workload):
            continue
        metadata = item.get("metadata") or {}
        spec = item.get("spec") or {}
        status = item.get("status") or {}
        rows.append(
            {
                "function": workload,
                "deployment": metadata.get("name", ""),
                "namespace": metadata.get("namespace", "default"),
                "desired_replicas": _number(spec.get("replicas", 0), "spec.replicas"),
                "ready_replicas": _number(status.get("readyReplicas", 0), "status.readyReplicas"),
                "available_replicas": _number(status.get("availableReplicas", 0), "status.availableReplicas"),
                "updated_replicas": _number(status.get("updatedReplicas", 0), "status.updatedReplicas"),
            }
        )
    if not rows:
        raise ValueError(f"no deployment matched workload {workload}")
    if len(rows) != 1:
        raise ValueError(f"expected exactly one deployment for {workload}, found {len(rows)}")
    rows.sort(key=lambda row: (str(row["namespace"]), str(row["deployment"])))
    for row in rows:
        if row["desired_replicas"] != expected or row["ready_replicas"] != expected:
            raise ValueError(
                f"{row['deployment']} admission mismatch: "
                f"desired={row['desired_replicas']} ready={row['ready_replicas']} expected={expected}"
            )
    aggregate_ready = sum(int(row["ready_replicas"]) for row in rows)
    aggregate_expected = len(rows) * expected
    if aggregate_ready != aggregate_expected:
        raise ValueError(
            f"aggregate admission mismatch: ready={aggregate_ready} expected={aggregate_expected}"
        )
    return rows


def write_evidence(path: Path, rows: list[dict[str, object]], expected: int) -> str:
    fields = (
        "function", "deployment", "namespace", "desired_replicas",
        "ready_replicas", "available_replicas", "updated_replicas",
    )
    with path.open("w", newline="", encoding="utf-8") as handle:
        writer = csv.DictWriter(handle, fieldnames=fields)
        writer.writeheader()
        writer.writerows(rows)
        writer.writerow(
            {
                "function": "__aggregate__",
                "deployment": "",
                "namespace": "",
                "desired_replicas": len(rows) * expected,
                "ready_replicas": sum(int(row["ready_replicas"]) for row in rows),
                "available_replicas": "",
                "updated_replicas": "",
            }
        )
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--deployments", required=True, type=Path)
    parser.add_argument("--workload", required=True)
    parser.add_argument("--expected-replicas", required=True, type=int)
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()
    try:
        if args.expected_replicas <= 0:
            raise ValueError("expected replicas must be positive")
        payload = json.loads(args.deployments.read_text(encoding="utf-8"))
        rows = validate(payload, args.workload, args.expected_replicas)
        digest = write_evidence(args.output, rows, args.expected_replicas)
    except (OSError, ValueError, json.JSONDecodeError) as error:
        print(f"admission_status=FAIL reason={error}")
        return 1
    aggregate = sum(int(row["ready_replicas"]) for row in rows)
    print("admission_status=PASS")
    print(f"admission_expected_replicas={args.expected_replicas}")
    print(f"admission_function_count={len(rows)}")
    print(f"admission_aggregate_expected_replicas={len(rows) * args.expected_replicas}")
    print(f"admission_aggregate_ready_replicas={aggregate}")
    print(f"admission_evidence_sha256={digest}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
