#!/usr/bin/env python3
"""Classify one started E3 loader acquisition from its duration evidence."""

from __future__ import annotations

import argparse
import csv
from decimal import Decimal
from pathlib import Path


def summarize(prefix: Path) -> tuple[int, int, Decimal]:
    matches = sorted(prefix.parent.glob(prefix.name + "_duration_*.csv"))
    if len(matches) != 1:
        raise ValueError(f"expected one duration evidence file, found {len(matches)}")
    path = matches[0]
    with path.open(newline="", encoding="utf-8") as handle:
        rows = list(csv.DictReader(handle))
    if not rows:
        raise ValueError(f"duration evidence contains no rows: {path}")
    successes = failures = 0
    for row in rows:
        value = row.get("success", "").strip().lower()
        if value == "true":
            successes += 1
        elif value == "false":
            failures += 1
        else:
            raise ValueError(f"duration evidence has invalid success value {value!r}")
    total = successes + failures
    fraction = Decimal(failures) / Decimal(total)
    if successes == 0:
        raise ValueError("zero successful invocations")
    return successes, failures, fraction


def validate(prefix: Path) -> tuple[int, int, Decimal]:
    successes, failures, fraction = summarize(prefix)
    # Strictly greater than 5% is a failure; exactly 5% is accepted.
    if failures * 100 > (successes + failures) * 5:
        raise ValueError(f"failure fraction {fraction} exceeds 0.05")
    return successes, failures, fraction


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--output-prefix", required=True, type=Path)
    # Kept as an optional provenance hook for callers that retain the loader
    # log alongside duration evidence; classification is based on the CSV.
    parser.add_argument("--loader-log", type=Path)
    args = parser.parse_args()
    try:
        successes, failures, fraction = summarize(args.output_prefix)
    except (OSError, ValueError) as error:
        print(f"evidence_status=FAIL scientific_status=FAILED reason={error}")
        return 1
    if failures * 100 > (successes + failures) * 5:
        print(
            "evidence_status=FAIL scientific_status=FAILED "
            f"reason=failure fraction {fraction} exceeds 0.05"
        )
        print(f"success_count={successes}")
        print(f"failure_count={failures}")
        print(f"failure_fraction={fraction}")
        return 1
    print("evidence_status=PASS scientific_status=ACCEPTED")
    print(f"success_count={successes}")
    print(f"failure_count={failures}")
    print(f"failure_fraction={fraction}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
