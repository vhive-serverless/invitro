#!/usr/bin/env python3
"""Generate and analyze the frozen B0 capacity sweep used by E2."""
from __future__ import annotations

import argparse
import csv
import math
import re
from collections import defaultdict
from pathlib import Path

from trace_modes import TRACE_MODES, trace_workload_name

WORKLOADS = (
    "helloworld", "chameleonserve", "cnnserve", "imageresize", "lrserving",
    "mapper", "pyaesserve", "reducer", "rnnserve", "streducer", "sttrainer",
)
STEPS = 20
SLO_MULTIPLIER = 5.0
FAILURE_THRESHOLD = 0.05
_MINUTE = re.compile(r"^min([0-9]+)\.inv[0-9]+$")


def read_averages(path: Path, *, require_complete: bool = True) -> dict[str, float]:
    with path.open(newline="", encoding="utf-8") as handle:
        rows = list(csv.DictReader(handle))
    required = {"workload", "unloaded_average_ms", "n_samples"}
    if not rows or not required.issubset(set(rows[0])):
        raise ValueError("E1 summary requires workload,unloaded_average_ms,n_samples")
    result: dict[str, float] = {}
    for row in rows:
        workload = row["workload"].strip()
        try:
            value = float(row["unloaded_average_ms"])
            samples = int(row["n_samples"])
        except ValueError as error:
            raise ValueError(f"invalid E1 row for {workload!r}") from error
        if workload in result or workload not in WORKLOADS or value <= 0 or samples <= 0:
            raise ValueError(f"invalid or duplicate E1 workload row: {workload!r}")
        result[workload] = value
    if require_complete and set(result) != set(WORKLOADS):
        raise ValueError(f"E1 workload set mismatch: missing={sorted(set(WORKLOADS)-set(result))} extra={sorted(set(result)-set(WORKLOADS))}")
    return result


def build_plan(averages: dict[str, float], cores: int, ceiling_multiplier: float = 1.0) -> dict[str, dict[str, object]]:
    if cores <= 0 or ceiling_multiplier <= 0:
        raise ValueError("worker cores and ceiling multiplier must be positive")
    result = {}
    for workload, average in averages.items():
        bound = math.floor(cores * 1000.0 * ceiling_multiplier / average)
        if bound < STEPS:
            raise ValueError(f"Rbound={bound} for {workload} cannot form {STEPS} distinct positive levels")
        levels = [math.floor(index * bound / STEPS) for index in range(1, STEPS + 1)]
        if len(set(levels)) != STEPS or levels[0] <= 0:
            raise ValueError(f"non-distinct calibration levels for {workload}: {levels}")
        result[workload] = {
            "unloaded_average_ms": average,
            "worker_cores": cores,
            "ceiling_multiplier": ceiling_multiplier,
            "rbound": bound,
            "levels": levels,
        }
    return result


def write_trace(plan: dict[str, object], workload: str, output: Path, warmup_minutes: int) -> None:
    if warmup_minutes != 2:
        raise ValueError("the frozen calibration requires exactly two warmup minutes")
    levels = list(plan["levels"])
    output.mkdir(parents=True, exist_ok=False)
    columns = [str(index) for index in range(-warmup_minutes, 0)] + [str(index) for index in range(1, STEPS + 1)]
    with (output / "invocations.csv").open("w", newline="", encoding="utf-8") as handle:
        writer = csv.DictWriter(handle, fieldnames=["FunctionName", *columns])
        writer.writeheader()
        row = {"FunctionName": workload}
        for offset, column in enumerate(columns[:warmup_minutes], start=1):
            row[column] = math.floor(levels[0] * 60 * offset / warmup_minutes)
        for column, rps in zip(columns[warmup_minutes:], levels):
            row[column] = rps * 60
        writer.writerow(row)
    with (output / "durations.csv").open("w", newline="", encoding="utf-8") as handle:
        writer = csv.DictWriter(handle, fieldnames=["FunctionName", "AvgDurationMs"])
        writer.writeheader()
        writer.writerow({"FunctionName": workload, "AvgDurationMs": plan["unloaded_average_ms"]})


def write_fixed_trace(average_ms: float, workload: str, mode: str, rps: int, output: Path,
                      warmup_minutes: int, measurement_minutes: int) -> None:
    if rps <= 0 or warmup_minutes < 0 or measurement_minutes <= 0:
        raise ValueError("fixed trace requires positive RPS/measurement and nonnegative warmup")
    trace_name = trace_workload_name(workload, mode)
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


def observe_duration(path: Path, workload: str, plan: dict[str, object]) -> list[dict[str, object]]:
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
        raise ValueError(f"{workload}: expected {STEPS} measured minutes, got {minute_ids}")
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
            "workload": workload,
            "step": len(observations) + 1,
            "rps": rps,
            "issued": len(rows),
            "successful": len(successes),
            "failed": len(rows) - len(successes),
            "failure_rate": (len(rows) - len(successes)) / len(rows) if rows else 1.0,
            "p99_ms": p99,
        })
    return observations


def analyze(plan: dict[str, dict[str, object]], observations: list[dict[str, str]]) -> list[dict[str, object]]:
    grouped: dict[str, list[dict[str, str]]] = defaultdict(list)
    for row in observations:
        grouped[row["workload"]].append(row)
    output = []
    for workload in WORKLOADS:
        item = plan[workload]
        rows = sorted(grouped.get(workload, []), key=lambda row: int(row["step"]))
        if len(rows) != STEPS or [int(row["rps"]) for row in rows] != item["levels"]:
            raise ValueError(f"{workload}: observation levels are missing, duplicated, or reordered")
        first_failure = None
        last_good = None
        for row in rows:
            failed = float(row["failure_rate"]) > FAILURE_THRESHOLD
            violates_slo = float(row["p99_ms"]) >= SLO_MULTIPLIER * float(item["unloaded_average_ms"])
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
            "workload": workload,
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


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--averages", required=True, type=Path)
    parser.add_argument("--cores", required=True, type=int)
    parser.add_argument("--ceiling-multiplier", type=float, default=1.0)
    sub = parser.add_subparsers(dest="command", required=True)
    plan_parser = sub.add_parser("plan")
    plan_parser.add_argument("--output", required=True, type=Path)
    trace_parser = sub.add_parser("trace")
    trace_parser.add_argument("--workload", required=True, choices=WORKLOADS)
    trace_parser.add_argument("--warmup-minutes", type=int, default=2)
    trace_parser.add_argument("--output", required=True, type=Path)
    observe_parser = sub.add_parser("observe")
    observe_parser.add_argument("--workload", required=True, choices=WORKLOADS)
    observe_parser.add_argument("--duration-csv", required=True, type=Path)
    observe_parser.add_argument("--output", required=True, type=Path)
    fixed_parser = sub.add_parser("fixed-trace")
    fixed_parser.add_argument("--workload", required=True, choices=WORKLOADS)
    fixed_parser.add_argument("--mode", required=True, choices=TRACE_MODES)
    fixed_parser.add_argument("--rps", required=True, type=int)
    fixed_parser.add_argument("--warmup-minutes", type=int, default=2)
    fixed_parser.add_argument("--measurement-minutes", type=int, default=3)
    fixed_parser.add_argument("--output", required=True, type=Path)
    finalize_parser = sub.add_parser("finalize")
    finalize_parser.add_argument("--observations", required=True, nargs="+", type=Path)
    finalize_parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()
    averages = read_averages(args.averages)
    plans = build_plan(averages, args.cores, args.ceiling_multiplier)
    if args.command == "plan":
        rows = [{"workload": workload, **{key: value for key, value in item.items() if key != "levels"},
                 **{f"rps_{index:02d}": value for index, value in enumerate(item["levels"], 1)}}
                for workload, item in plans.items()]
        write_rows(args.output, rows)
    elif args.command == "trace":
        write_trace(plans[args.workload], args.workload, args.output, args.warmup_minutes)
    elif args.command == "observe":
        write_rows(args.output, observe_duration(args.duration_csv, args.workload, plans[args.workload]))
    elif args.command == "fixed-trace":
        write_fixed_trace(averages[args.workload], args.workload, args.mode, args.rps, args.output,
                          args.warmup_minutes, args.measurement_minutes)
    else:
        rows = []
        for path in args.observations:
            with path.open(newline="", encoding="utf-8") as handle:
                rows.extend(csv.DictReader(handle))
        write_rows(args.output, analyze(plans, rows))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
