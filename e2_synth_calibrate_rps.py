#!/usr/bin/env python3
"""Generate and analyze payload-keyed InVM-Py sweeps for E2-Synth."""
from __future__ import annotations

import argparse
import csv
import math
import re
import sys
from collections import defaultdict
from pathlib import Path

from e2_synth_modes import MODES, PAYLOADS, workload_name

STEPS = 20
SLO_MULTIPLIER = 5.0
FAILURE_THRESHOLD = 0.05
_MINUTE = re.compile(r"^min([0-9]+)\.inv[0-9]+$")
_WARM = re.compile(r"(?:^|,\s*)warm_e2e_ns:\s*([0-9]+)(?:,|$)")


def extract_e1_unloaded(root: Path) -> list[dict[str, object]]:
    rows = []
    for payload in PAYLOADS:
        matches = sorted(root.rglob(f"invm-py_synthetic_e_0_p_{payload}_latency/event.csv"))
        if len(matches) != 1:
            raise ValueError(f"payload {payload}: expected one InVM-Py event.csv under {root}, got {len(matches)}")
        values: list[int] = []
        with matches[0].open(newline="", encoding="utf-8") as handle:
            for row in csv.DictReader(handle):
                match = _WARM.search(row.get("event", ""))
                if match:
                    values.append(int(match.group(1)))
        if not values:
            raise ValueError(f"payload {payload}: no warm_e2e_ns values in {matches[0]}")
        rows.append({"payload_bytes": payload,
                     "unloaded_average_ms": sum(values) / len(values) / 1_000_000.0,
                     "n_samples": len(values)})
    return rows


def merge_references(paths: list[Path]) -> list[dict[str, str]]:
    if len(paths) != 2:
        raise ValueError("exactly two calibration partials are required")
    rows: list[dict[str, str]] = []
    fieldnames: list[str] | None = None
    for path in paths:
        with path.open(newline="", encoding="utf-8") as handle:
            reader = csv.DictReader(handle)
            current = list(reader)
            if len(current) != 5:
                raise ValueError(f"{path}: expected five calibration rows")
            if fieldnames is None:
                fieldnames = list(reader.fieldnames or ())
            elif fieldnames != list(reader.fieldnames or ()):
                raise ValueError("calibration partial schemas differ")
            rows.extend(current)
    by_payload: dict[int, dict[str, str]] = {}
    for row in rows:
        payload = int(row["payload_bytes"])
        if payload in by_payload or payload not in PAYLOADS:
            raise ValueError(f"duplicate or unsupported merged payload {payload}")
        if row.get("status") not in ("BOUNDARY_OBSERVED", "RIGHT_CENSORED") or not row.get("rref"):
            raise ValueError(f"payload {payload} has no admissible frozen reference")
        by_payload[payload] = row
    if set(by_payload) != set(PAYLOADS):
        raise ValueError("merged calibration does not cover the canonical ten payloads")
    invariant = ("worker_cores", "ceiling_multiplier")
    for key in invariant:
        if len({row[key] for row in rows}) != 1:
            raise ValueError(f"calibration contract mismatch in {key}")
    return [by_payload[payload] for payload in PAYLOADS]


def _row_payload(row: dict[str, str]) -> int:
    text = row.get("payload_bytes", row.get("payload", "")).strip()
    if not text:
        raise ValueError("E1-Synth summary row has no payload_bytes")
    return int(text)


def read_averages(path: Path, *, require_complete: bool = True) -> dict[int, float]:
    with path.open(newline="", encoding="utf-8") as handle:
        rows = list(csv.DictReader(handle))
    required = {"unloaded_average_ms", "n_samples"}
    if not rows or not required.issubset(set(rows[0])) or not ({"payload", "payload_bytes"} & set(rows[0])):
        raise ValueError("E1-Synth summary requires payload_bytes,unloaded_average_ms,n_samples")
    result: dict[int, float] = {}
    for row in rows:
        try:
            payload = _row_payload(row)
            value = float(row["unloaded_average_ms"])
            samples = int(row["n_samples"])
        except ValueError as error:
            raise ValueError("invalid E1-Synth summary row") from error
        if payload in result or payload not in PAYLOADS or not math.isfinite(value) or value <= 0 or samples <= 0:
            raise ValueError(f"invalid or duplicate E1-Synth payload row: {payload}")
        result[payload] = value
    if require_complete and set(result) != set(PAYLOADS):
        raise ValueError(f"E1-Synth payload set mismatch: missing={sorted(set(PAYLOADS)-set(result))} extra={sorted(set(result)-set(PAYLOADS))}")
    return result


def build_plan(averages: dict[int, float], cores: int, ceiling_multiplier: float = 1.0,
               selected: tuple[int, ...] | list[int] | None = None) -> dict[int, dict[str, object]]:
    if cores <= 0 or ceiling_multiplier <= 0:
        raise ValueError("worker cores and ceiling multiplier must be positive")
    result = {}
    for payload in (selected if selected is not None else PAYLOADS):
        average = averages[payload]
        bound = math.floor(cores * 1000.0 * ceiling_multiplier / average)
        if bound < STEPS:
            raise ValueError(f"Rbound={bound} for payload {payload} cannot form {STEPS} distinct positive levels")
        levels = [math.floor(index * bound / STEPS) for index in range(1, STEPS + 1)]
        if len(set(levels)) != STEPS or levels[0] <= 0:
            raise ValueError(f"non-distinct calibration levels for payload {payload}: {levels}")
        result[payload] = {
            "unloaded_average_ms": average,
            "worker_cores": cores,
            "ceiling_multiplier": ceiling_multiplier,
            "rbound": bound,
            "levels": levels,
        }
    return result


def write_trace(plan: dict[str, object], payload: int, output: Path, warmup_minutes: int) -> None:
    if warmup_minutes != 2:
        raise ValueError("the frozen calibration requires exactly two warmup minutes")
    levels = list(plan["levels"])
    output.mkdir(parents=True, exist_ok=False)
    columns = [str(index) for index in range(-warmup_minutes, 0)] + [str(index) for index in range(1, STEPS + 1)]
    with (output / "invocations.csv").open("w", newline="", encoding="utf-8") as handle:
        writer = csv.DictWriter(handle, fieldnames=["FunctionName", *columns])
        writer.writeheader()
        row = {"FunctionName": workload_name(payload, "invm-py")}
        for offset, column in enumerate(columns[:warmup_minutes], start=1):
            row[column] = math.floor(levels[0] * 60 * offset / warmup_minutes)
        for column, rps in zip(columns[warmup_minutes:], levels):
            row[column] = rps * 60
        writer.writerow(row)
    with (output / "durations.csv").open("w", newline="", encoding="utf-8") as handle:
        writer = csv.DictWriter(handle, fieldnames=["FunctionName", "AvgDurationMs"])
        writer.writeheader()
        writer.writerow({"FunctionName": workload_name(payload, "invm-py"), "AvgDurationMs": plan["unloaded_average_ms"]})


def write_fixed_trace(average_ms: float, payload: int, mode: str, rps: int, output: Path,
                      warmup_minutes: int, measurement_minutes: int) -> None:
    if rps <= 0 or warmup_minutes < 0 or measurement_minutes <= 0:
        raise ValueError("fixed trace requires positive RPS/measurement and nonnegative warmup")
    trace_name = workload_name(payload, mode)
    output.mkdir(parents=True, exist_ok=False)
    columns = [str(index) for index in range(-warmup_minutes, 0)] + [str(index) for index in range(1, measurement_minutes + 1)]
    with (output / "invocations.csv").open("w", newline="", encoding="utf-8") as handle:
        writer = csv.DictWriter(handle, fieldnames=["FunctionName", *columns])
        writer.writeheader()
        row = {"FunctionName": trace_name}
        for offset, column in enumerate(columns[:warmup_minutes], start=1):
            row[column] = math.floor(rps * 60 * offset / max(1, warmup_minutes))
        for column in columns[warmup_minutes:]:
            row[column] = rps * 60
        writer.writerow(row)
    with (output / "durations.csv").open("w", newline="", encoding="utf-8") as handle:
        writer = csv.DictWriter(handle, fieldnames=["FunctionName", "AvgDurationMs"])
        writer.writeheader()
        writer.writerow({"FunctionName": trace_name, "AvgDurationMs": average_ms})


def _percentile(values: list[float], quantile: float) -> float:
    values = sorted(values)
    position = (len(values) - 1) * quantile
    lower, upper = math.floor(position), math.ceil(position)
    if lower == upper:
        return values[lower]
    return values[lower] + (values[upper] - values[lower]) * (position - lower)


def observe_duration(path: Path, payload: int, plan: dict[str, object]) -> list[dict[str, object]]:
    buckets: dict[int, list[dict[str, str]]] = defaultdict(list)
    with path.open(newline="", encoding="utf-8") as handle:
        reader = csv.DictReader(handle)
        required = {"phase", "invocationID", "responseTime", "success"}
        if not required.issubset(set(reader.fieldnames or ())):
            raise ValueError(f"duration CSV lacks E2 fields: {sorted(required)}")
        for row in reader:
            match = _MINUTE.fullmatch(row["invocationID"])
            if row["phase"] != "2" or match is None:
                continue
            buckets[int(match.group(1))].append(row)
    minute_ids = sorted(buckets)
    if len(minute_ids) != STEPS:
        raise ValueError(f"payload {payload}: expected {STEPS} measured minutes, got {minute_ids}")
    observations = []
    for minute_id, rps in zip(minute_ids, plan["levels"]):
        rows = buckets[minute_id]
        successes = [row for row in rows if row["success"].lower() == "true"]
        latencies = [float(row["responseTime"]) / 1000.0 for row in successes]
        if not rows or not latencies:
            p99 = math.inf
        else:
            p99 = _percentile(latencies, 0.99)
        observations.append({
            "payload_bytes": payload,
            "step": len(observations) + 1,
            "rps": rps,
            "issued": len(rows),
            "successful": len(successes),
            "failed": len(rows) - len(successes),
            "failure_rate": (len(rows) - len(successes)) / len(rows) if rows else 1.0,
            "p99_ms": p99,
        })
    return observations


def analyze(plan: dict[int, dict[str, object]], observations: list[dict[str, str]],
            selected: list[int], cluster_id: str,
            slo_multiplier: float = SLO_MULTIPLIER,
            failure_threshold: float = FAILURE_THRESHOLD) -> list[dict[str, object]]:
    grouped: dict[int, list[dict[str, str]]] = defaultdict(list)
    for row in observations:
        grouped[int(row["payload_bytes"])].append(row)
    output = []
    for payload in selected:
        item = plan[payload]
        rows = sorted(grouped.get(payload, []), key=lambda row: int(row["step"]))
        if len(rows) != STEPS or [int(row["rps"]) for row in rows] != item["levels"]:
            raise ValueError(f"payload {payload}: observation levels are missing, duplicated, or reordered")
        first_failure = None
        last_good = None
        for row in rows:
            failed = float(row["failure_rate"]) > failure_threshold
            violates_slo = float(row["p99_ms"]) >= slo_multiplier * float(item["unloaded_average_ms"])
            if failed or violates_slo:
                first_failure = row
                break
            last_good = row
        if first_failure is None:
            # The campaign is intentionally single pass.  Preserve the fact
            # that the true boundary was not observed, but still provide a
            # conservative operational load for the downstream matched cells.
            # This value is not an estimate of 50% of maximum throughput.
            status, rmax = "RIGHT_CENSORED", ""
            rref = math.floor(int(rows[-1]["rps"]) / 2)
            first_step, first_rps = "", ""
            reference_kind = "RIGHT_CENSORED_REFERENCE"
        elif last_good is None:
            status, rmax, rref = "NO_ADMISSIBLE_LEVEL", "", ""
            first_step, first_rps = first_failure["step"], first_failure["rps"]
            reference_kind = "NO_REFERENCE"
        else:
            status = "BOUNDARY_OBSERVED"
            rmax = int(last_good["rps"])
            rref = math.floor(rmax / 2)
            first_step, first_rps = first_failure["step"], first_failure["rps"]
            reference_kind = "OBSERVED_BOUNDARY_REFERENCE"
        output.append({
            "payload_bytes": payload,
            "calibration_cluster": cluster_id,
            "unloaded_average_ms": item["unloaded_average_ms"],
            "worker_cores": item["worker_cores"],
            "ceiling_multiplier": item["ceiling_multiplier"],
            "rbound": item["rbound"],
            "first_failing_step": first_step,
            "first_failing_rps": first_rps,
            "rmax_b0": rmax,
            "rref": rref,
            "status": status,
            "reference_kind": reference_kind,
        })
    return output


def write_rows(path: Path, rows: list[dict[str, object]]) -> None:
    if not rows:
        raise ValueError("refusing empty output")
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("w", newline="", encoding="utf-8") as handle:
        writer = csv.DictWriter(handle, fieldnames=list(rows[0]))
        writer.writeheader()
        writer.writerows(rows)


def parse_payloads(text: str) -> list[int]:
    result: list[int] = []
    for raw in text.split(","):
        payload = int(raw)
        if payload not in PAYLOADS or payload in result:
            raise ValueError(f"unsupported or duplicate E2-Synth payload: {payload}")
        result.append(payload)
    if not result:
        raise ValueError("payload selection is empty")
    return result


def main() -> int:
    if len(sys.argv) > 1 and sys.argv[1] == "extract-e1":
        special = argparse.ArgumentParser()
        special.add_argument("command")
        special.add_argument("--input-root", required=True, type=Path)
        special.add_argument("--output", required=True, type=Path)
        args = special.parse_args()
        write_rows(args.output, extract_e1_unloaded(args.input_root))
        return 0
    if len(sys.argv) > 1 and sys.argv[1] == "merge":
        special = argparse.ArgumentParser()
        special.add_argument("command")
        special.add_argument("--partials", required=True, nargs=2, type=Path)
        special.add_argument("--output", required=True, type=Path)
        args = special.parse_args()
        write_rows(args.output, merge_references(args.partials))
        return 0
    parser = argparse.ArgumentParser()
    parser.add_argument("--averages", required=True, type=Path)
    parser.add_argument("--cores", required=True, type=int)
    parser.add_argument("--ceiling-multiplier", type=float, default=1.0)
    sub = parser.add_subparsers(dest="command", required=True)
    plan_parser = sub.add_parser("plan")
    plan_parser.add_argument("--payloads", required=True)
    plan_parser.add_argument("--steps", type=int, default=STEPS)
    plan_parser.add_argument("--minutes-per-step", type=int, default=1)
    plan_parser.add_argument("--output", required=True, type=Path)
    trace_parser = sub.add_parser("trace")
    trace_parser.add_argument("--payload", required=True, type=int, choices=PAYLOADS)
    trace_parser.add_argument("--warmup-minutes", type=int, default=2)
    trace_parser.add_argument("--steps", type=int, default=STEPS)
    trace_parser.add_argument("--minutes-per-step", type=int, default=1)
    trace_parser.add_argument("--output", required=True, type=Path)
    observe_parser = sub.add_parser("observe")
    observe_parser.add_argument("--payload", required=True, type=int, choices=PAYLOADS)
    observe_parser.add_argument("--duration-csv", required=True, type=Path)
    observe_parser.add_argument("--failure-threshold", type=float, default=FAILURE_THRESHOLD)
    observe_parser.add_argument("--slo-multiplier", type=float, default=SLO_MULTIPLIER)
    observe_parser.add_argument("--steps", type=int, default=STEPS)
    observe_parser.add_argument("--minutes-per-step", type=int, default=1)
    observe_parser.add_argument("--output", required=True, type=Path)
    fixed_parser = sub.add_parser("fixed-trace")
    fixed_parser.add_argument("--payload", required=True, type=int, choices=PAYLOADS)
    fixed_parser.add_argument("--mode", required=True, choices=MODES)
    fixed_parser.add_argument("--rps", required=True, type=int)
    fixed_parser.add_argument("--warmup-minutes", type=int, default=2)
    fixed_parser.add_argument("--measurement-minutes", type=int, default=3)
    fixed_parser.add_argument("--output", required=True, type=Path)
    finalize_parser = sub.add_parser("finalize")
    finalize_parser.add_argument("--payloads", required=True)
    finalize_parser.add_argument("--cluster-id", required=True)
    finalize_parser.add_argument("--observations", required=True, nargs="+", type=Path)
    finalize_parser.add_argument("--failure-threshold", type=float, default=FAILURE_THRESHOLD)
    finalize_parser.add_argument("--slo-multiplier", type=float, default=SLO_MULTIPLIER)
    finalize_parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()
    averages = read_averages(args.averages)
    if hasattr(args, "steps") and (args.steps != STEPS or args.minutes_per_step != 1):
        raise ValueError("E2-Synth calibration requires exactly 20 one-minute levels")
    selected = parse_payloads(args.payloads) if hasattr(args, "payloads") else [args.payload]
    plans = build_plan(averages, args.cores, args.ceiling_multiplier, selected)
    if args.command == "plan":
        rows = [{"payload_bytes": payload, **{key: value for key, value in item.items() if key != "levels"},
                 **{f"rps_{index:02d}": value for index, value in enumerate(item["levels"], 1)}}
                for payload, item in plans.items()]
        write_rows(args.output, rows)
    elif args.command == "trace":
        write_trace(plans[args.payload], args.payload, args.output, args.warmup_minutes)
    elif args.command == "observe":
        if not (0 <= args.failure_threshold < 1) or args.slo_multiplier <= 0:
            raise ValueError("invalid failure/SLO threshold")
        write_rows(args.output, observe_duration(args.duration_csv, args.payload, plans[args.payload]))
    elif args.command == "fixed-trace":
        write_fixed_trace(averages[args.payload], args.payload, args.mode, args.rps, args.output,
                          args.warmup_minutes, args.measurement_minutes)
    else:
        rows = []
        for path in args.observations:
            with path.open(newline="", encoding="utf-8") as handle:
                rows.extend(csv.DictReader(handle))
        write_rows(args.output, analyze(plans, rows, selected, args.cluster_id,
                                        args.slo_multiplier, args.failure_threshold))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
