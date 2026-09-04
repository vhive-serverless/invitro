#!/usr/bin/env python3
"""Validate dry E2-Synth plan output against a declared matrix slice."""
import argparse
from pathlib import Path

MODES = ("invm-py", "invm-js", "invm-go", "hosttcp-go", "nexus-py",
         "nexus-js", "nexus-go", "nexus-rdma-py", "nexus-rdma-go")
PAYLOADS = (4, 4096, 16384, 65536, 262144, 1048576, 2097152,
            4194304, 8388608, 16777216)
CURRENT = PAYLOADS[::2]
SUPPLIED = PAYLOADS[1::2]


def parse(path: Path):
    rows = []
    for line in path.read_text(encoding="utf-8").splitlines():
        if not line.startswith("CELL "):
            continue
        row = dict(field.split("=", 1) for field in line.split()[1:] if "=" in field)
        rows.append(row)
    return rows


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("plan", type=Path)
    parser.add_argument("--kind", required=True,
                        choices=("full", "smoke", "current", "supplied", "cal-current", "cal-supplied"))
    args = parser.parse_args()
    rows = parse(args.plan)
    calibration = args.kind.startswith("cal-")
    if args.kind == "full": payloads = PAYLOADS
    elif args.kind == "smoke": payloads = (65536, 16777216)
    elif args.kind in ("current", "cal-current"): payloads = CURRENT
    else: payloads = SUPPLIED
    modes = ("invm-py",) if calibration else MODES
    expected = {(mode, payload) for payload in payloads for mode in modes}
    observed = [(row.get("mode"), int(row.get("payload_bytes", "-1"))) for row in rows]
    if len(observed) != len(expected) or set(observed) != expected:
        raise SystemExit(f"matrix mismatch rows={len(observed)} unique={len(set(observed))}")
    if not calibration:
        for index, payload in enumerate(payloads):
            got = [mode for mode, candidate in observed if candidate == payload]
            want = list(MODES if index % 2 == 0 else reversed(MODES))
            if got != want:
                raise SystemExit(f"payload {payload}: mode order {got}, want {want}")
    print(f"PLAN_PASS kind={args.kind} cells={len(rows)} unique={len(set(observed))}")


if __name__ == "__main__":
    main()
