#!/usr/bin/env python3
"""
Generate sweep-style function invocation traces from a reference CSV.

For each workload, one representative function is selected from the reference
trace at the scaled-down per-function load `(rps / divisor)`.

The scale arguments describe how many function rows should be active over time:

  - `start_scale`: number of functions active at the first timestamp
  - `end_scale`: total number of function rows to emit per workload
  - `step`: number of additional functions activated at each later timestamp

Examples:

  - `--start-scale 1 --end-scale 5 --step 1` activates functions as
    `1, 2, 3, 4, 5, 5, ...`
  - `--start-scale 1 --end-scale 5 --step 2` activates functions as
    `1, 3, 5, 5, ...`
  - `--start-scale 10 --end-scale 10 --step 1` emits 10 concurrent functions
    that all start at the first timestamp

Each function row may also be rotated by `shift_step` to vary the selected
trace. Functions that are not yet active receive zero requests before their
activation timestamp. Warmup ramps are generated only for rows that are already
active when the experiment begins.

The CLI remains compatible with generate_scaled_trace.py so it can
drop-in replace that call in shell scripts:

  python3 generate_trace_sweep.py \\
      --divisor $divisor \\
      --start-scale $START_SCALE \\
      --end-scale $END_SCALE \\
      --step $STEP \\
      --warmup-duration $EXPWARMUP \\
      --warmup-scale 1
"""

from __future__ import annotations

import argparse
import json
import sys
from dataclasses import dataclass, field
from pathlib import Path
from typing import Dict, List

import pandas as pd

# --- Default Configuration ---

DEFAULT_INPUT = "data/traces/reference/preprocessed_150/invocations.csv"
DEFAULT_OUTPUT_DIR = "data/traces/nexus"


# # RPS that drives load to 50% CPU utilization
# DEFAULT_WORKLOAD_RPS: Dict[str, float] = {
#     "chameleonserve": 795, "cnnserve": 100, "imageresize": 30, "lrserving": 680,
#     "mapper": 65, "pyaesserve": 1155, "reducer": 15, "rnnserve": 240,
#     "streducer": 225, "sttrainer": 180
# }

# 50% of max RPS the system can handle
DEFAULT_WORKLOAD_RPS: Dict[str, float] = {
    "chameleonserve": 510, "cnnserve": 75, "imageresize": 26, "lrserving": 475,
    "mapper": 60, "pyaesserve": 500, "reducer": 12, "rnnserve": 90, 
    "streducer": 160, "sttrainer": 130
}

DEFAULT_WORKLOAD_AVG_DURATION_MS: Dict[str, float] = {
    "chameleonserve": 18.26, "cnnserve": 165.387, "imageresize": 491.001,
	"lrserving": 28.387, "mapper": 245.188, "pyaesserve": 12.862,
	"reducer": 1025.913, "rnnserve": 2 * 25.4895, "streducer": 88.839,
	"sttrainer": 54.328,
}

# --- Configuration Class ---

@dataclass
class Config:
    """Configuration for the sweep trace generator."""
    input_path: Path
    output_dir: Path
    workload_rps: Dict[str, float] = field(default_factory=lambda: DEFAULT_WORKLOAD_RPS)
    divisor: float = 10.0
    start_scale: float = 1.0
    end_scale: float = 10.0
    step: float = 1.0
    shift_step: int = 1
    warmup_duration: int = 0
    warmup_scale: float = 1.0
    min_divisor: float = 10.0
    max_multiplier: float = 2.0
    name_suffix: str = ""
    dry_run: bool = False


# --- Core Logic Class ---

class SweepTraceBuilder:
    """Build invocation traces from per-workload activation schedules."""

    def __init__(self, config: Config):
        self.config = config
        self.base_df = self._load_base_trace()
        self.time_cols = self._get_time_columns(self.base_df)
        self.stats_df = self._compute_invocation_stats()

    # --- Public entry point ---

    def run(self) -> tuple[pd.DataFrame, pd.DataFrame]:
        """Execute the sweep trace building pipeline."""
        function_count = self._compute_function_count()
        start_count, _, step_count = self._get_scale_counts()
        max_activation_offset = self._compute_max_activation_offset(function_count)

        print(
            f"[INFO] Building sweep trace: {function_count} rows per workload "
            f"({start_count} active at start, +{step_count} per step) over "
            f"{len(self.time_cols)} time columns."
        )

        if max_activation_offset >= len(self.time_cols):
            raise ValueError(
                f"Activation offset ({max_activation_offset}) exceeds the usable range of the "
                f"reference trace ({len(self.time_cols)} time columns). "
                "Reduce --end-scale, increase --step, or increase --start-scale."
            )

        invocations_df = self._build_invocations(function_count)
        durations_df = self._build_durations(invocations_df)
        return invocations_df, durations_df

    # --- Internal helpers ---

    def _get_scale_counts(self) -> tuple[int, int, int]:
        return int(self.config.start_scale), int(self.config.end_scale), int(self.config.step)

    def _compute_function_count(self) -> int:
        _, end_count, _ = self._get_scale_counts()
        return end_count

    def _compute_activation_offset(self, function_index: int) -> int:
        start_count, _, step_count = self._get_scale_counts()
        if function_index < start_count:
            return 0
        return ((function_index - start_count) // step_count) + 1

    def _compute_max_activation_offset(self, function_count: int) -> int:
        if function_count <= 0:
            return 0
        return self._compute_activation_offset(function_count - 1)

    def _load_base_trace(self) -> pd.DataFrame:
        """Load and pre-filter the base invocation trace CSV."""
        print(f"[INFO] Loading base invocation trace: {self.config.input_path}")
        df = pd.read_csv(self.config.input_path)
        df.columns = df.columns.map(str)
        # Keep only HTTP-triggered functions (matches generate_trace.py behaviour)
        return df[df["Trigger"] == "http"].copy()

    @staticmethod
    def _get_time_columns(df: pd.DataFrame) -> List[str]:
        """Return columns whose names are pure decimal integers (e.g. '540'..'689')."""
        cols = [c for c in df.columns if isinstance(c, str) and c.isdigit()]
        if not cols:
            raise ValueError("No numbered time-series columns detected in base trace.")
        return cols

    def _compute_invocation_stats(self) -> pd.DataFrame:
        """Compute per-row invocation statistics over the time-series columns."""
        print("[INFO] Computing per-function statistics...")
        df = self.base_df
        ts = df[self.time_cols]

        stats = pd.DataFrame(index=df.index)
        stats["invocation_count_sum"] = ts.sum(axis=1)
        stats["invocation_count_avg"] = ts.mean(axis=1)
        stats["invocation_count_max"] = ts.max(axis=1)
        stats["invocation_count_min"] = ts.min(axis=1)

        meta_cols = [c for c in ["HashOwner", "HashApp", "HashFunction", "Trigger"] if c in df.columns]
        result = pd.concat([df[meta_cols], stats, ts], axis=1)
        return result.sort_values(by="invocation_count_sum", ascending=False)

    def _select_closest_function(self, target_rps: float) -> pd.Series:
        """
        Select the reference function whose average invocation rate (RPM) is
        closest to `target_rps * 60`, subject to min/max bounds.
        """
        target_rpm = target_rps * 60.0
        min_rpm_bound = target_rpm / self.config.min_divisor
        max_rpm_bound = target_rpm * self.config.max_multiplier

        candidates = self.stats_df[
            (self.stats_df["invocation_count_max"] < max_rpm_bound) &
            (self.stats_df["invocation_count_min"] > min_rpm_bound)
        ]

        if candidates.empty:
            raise ValueError(
                f"No reference function found for target_rps={target_rps:.2f} "
                f"(target_rpm={target_rpm:.1f}, "
                f"bounds=[{min_rpm_bound:.1f}, {max_rpm_bound:.1f}]). "
                "Try adjusting --divisor, --min-divisor, or --max-multiplier."
            )

        diff = (candidates["invocation_count_avg"] - target_rpm).abs()
        return candidates.loc[diff.idxmin()].copy()

    def _build_invocations(self, function_count: int) -> pd.DataFrame:
        """
        Build the full invocations dataframe.

        For each workload:
          - Select one representative function from the reference trace.
          - Emit `end_scale` rows in total.
          - Rows 0..start_scale-1 are active at the first timestamp.
          - Remaining rows activate in batches of `step` at later timestamps.
          - Only rows active at timestamp 0 receive a warmup ramp.
        """
        warmup_cols = (
            [str(i) for i in range(-self.config.warmup_duration, 0)]
            if self.config.warmup_duration > 0
            else []
        )
        col_order = ["FunctionName"] + warmup_cols + self.time_cols

        all_rows: List[dict] = []

        for workload, rps in self.config.workload_rps.items():
            target_rps = rps / self.config.divisor
            print(
                f"[INFO] {workload}: RPS={rps}, per-function selection target="
                f"{target_rps:.2f} RPS ({target_rps * 60:.1f} RPM)"
            )

            base_fn = self._select_closest_function(target_rps)
            base_values: List[int] = [int(v) for v in base_fn[self.time_cols].tolist()]
            trace_length = len(base_values)
            workload_name = f"{workload}{self.config.name_suffix}"

            for i in range(function_count):
                row: dict = {"FunctionName": workload_name}
                activation_offset = self._compute_activation_offset(i)

                # Rotate the trace by shift_step * i so each row has independent fluctuation
                shift = (self.config.shift_step * i) % trace_length if trace_length > 0 else 0
                shifted_values = base_values[shift:] + base_values[:shift]

                # Warmup columns
                if warmup_cols:
                    if activation_offset == 0:
                        # Warm each row that is active at experiment start.
                        first_val = float(shifted_values[0]) * self.config.warmup_scale
                        for k, col in enumerate(warmup_cols, start=1):
                            row[col] = int(first_val * k / self.config.warmup_duration)
                    else:
                        # Rows activated later should stay idle during warmup.
                        for col in warmup_cols:
                            row[col] = 0

                # Rows become active according to the start/end/step schedule.
                emitted_values = [0] * activation_offset + shifted_values[activation_offset:]
                for col, val in zip(self.time_cols, emitted_values):
                    row[col] = val

                all_rows.append(row)

        return pd.DataFrame(all_rows)[col_order]

    def _build_durations(self, invocations_df: pd.DataFrame) -> pd.DataFrame:
        """Build duration dataframe with one entry per row in invocations (including duplicates)."""
        rows: List[dict] = []
        for func_name in invocations_df["FunctionName"]:
            base_workload = func_name.split("-")[0]
            duration_ms = DEFAULT_WORKLOAD_AVG_DURATION_MS.get(base_workload, 1000.0)
            rows.append({"FunctionName": func_name, "AvgDurationMs": duration_ms})
        return pd.DataFrame(rows)


# --- CLI and Main Execution ---

def parse_workload_rps_arg(value: str) -> Dict[str, float]:
    """Parse workload RPS mapping from a JSON file path or an inline JSON string."""
    p = Path(value)
    try:
        if p.exists():
            with p.open("r", encoding="utf-8") as f:
                return json.load(f)
        return json.loads(value)
    except Exception as e:
        raise argparse.ArgumentTypeError(f"Invalid workload RPS mapping '{value}': {e}")


def build_arg_parser() -> argparse.ArgumentParser:
    """Build the command-line argument parser."""
    parser = argparse.ArgumentParser(
        description="Generate staggered sweep invocation traces from a reference CSV.",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  # Start with 1 active function and add 1 per step until there are 15
  %(prog)s --divisor 100 --start-scale 1 --end-scale 15 --step 1 \\
           --warmup-duration 2 --warmup-scale 1

  # Emit 10 concurrent functions that all start at timestamp 0
  %(prog)s --divisor 100 --start-scale 10 --end-scale 10 --step 1 \\
           --warmup-duration 2 --warmup-scale 1

  # Dry-run a schedule with 1 active function at start and +2 per step
  %(prog)s --divisor 100 --start-scale 1 --end-scale 5 --step 2 \\
           --warmup-duration 2 --warmup-scale 1 --dry-run

  # With naming suffixes
  %(prog)s --divisor 100 --start-scale 1 --end-scale 15 --step 1 \\
           --warmup-duration 2 --warmup-scale 1 --s3 --rpc
        """,
    )

    # Required scaling arguments (mirror generate_scaled_trace.py)
    parser.add_argument("--divisor", type=float, required=True,
                        help="Divide workload RPS by this to get per-instance target for function selection.")
    parser.add_argument("--start-scale", type=float, required=True,
                        help="Number of functions active at the first timestamp.")
    parser.add_argument("--end-scale", type=float, required=True,
                        help="Total number of function rows to emit per workload.")
    parser.add_argument("--step", type=float, required=True,
                        help="Number of additional functions activated at each later timestamp.")
    parser.add_argument("--shift-step", type=int, default=1,
                        help="Column offset between consecutive staggered rows. Default: 1")

    # Optional arguments
    parser.add_argument("--input", default=DEFAULT_INPUT,
                        help=f"Path to reference invocations CSV. Default: {DEFAULT_INPUT}")
    parser.add_argument("--output", default=DEFAULT_OUTPUT_DIR,
                        help=f"Directory to write output CSVs. Default: {DEFAULT_OUTPUT_DIR}")
    parser.add_argument("--workload-rps", type=parse_workload_rps_arg,
                        default=DEFAULT_WORKLOAD_RPS,
                        help="Workload->RPS mapping as a JSON file path or inline JSON.")
    parser.add_argument("--warmup-duration", type=int, default=0,
                        help="Warmup phase length in minutes (prepended columns). Default: 0")
    parser.add_argument("--warmup-scale", type=float, default=1.0,
                        help="Warmup ramp target as a fraction of the first column's value. Default: 1.0")
    parser.add_argument("--min-divisor", type=float, default=10.0,
                        help="Lower-bound filter divisor for function selection. Default: 10.0")
    parser.add_argument("--max-multiplier", type=float, default=2.0,
                        help="Upper-bound filter multiplier for function selection. Default: 2.0")
    parser.add_argument("--s3", action="store_true",
                        help="Append '-s3' suffix to workload names.")
    parser.add_argument("--rpc", action="store_true",
                        help="Append '-rpc' suffix to workload names.")
    parser.add_argument("--dry-run", action="store_true",
                        help="Run all steps but do not write output files.")

    return parser


def main(argv: List[str] | None = None) -> int:
    """Main execution function."""
    parser = build_arg_parser()
    args = parser.parse_args(list(argv) if argv is not None else None)

    # Basic validation
    if not Path(args.input).exists():
        print(f"[ERROR] Input file not found: {args.input}", file=sys.stderr)
        return 1

    if args.divisor <= 0:
        print("[ERROR] --divisor must be > 0", file=sys.stderr)
        return 1

    for flag, value in (
        ("--start-scale", args.start_scale),
        ("--end-scale", args.end_scale),
        ("--step", args.step),
    ):
        if not value.is_integer():
            print(f"[ERROR] {flag} must be an integer function count", file=sys.stderr)
            return 1

    if args.start_scale < 0:
        print("[ERROR] --start-scale must be >= 0", file=sys.stderr)
        return 1

    if args.end_scale <= 0:
        print("[ERROR] --end-scale must be > 0", file=sys.stderr)
        return 1

    if args.start_scale > args.end_scale:
        print("[ERROR] --start-scale must be <= --end-scale", file=sys.stderr)
        return 1

    if args.step <= 0:
        print("[ERROR] --step must be > 0", file=sys.stderr)
        return 1

    # Build name suffix (order: -s3 before -rpc)
    suffix_parts = []
    if args.s3:
        suffix_parts.append("s3")
    if args.rpc:
        suffix_parts.append("rpc")
    name_suffix = f"-{'-'.join(suffix_parts)}" if suffix_parts else ""

    try:
        config = Config(
            input_path=Path(args.input),
            output_dir=Path(args.output),
            workload_rps=args.workload_rps,
            divisor=args.divisor,
            start_scale=args.start_scale,
            end_scale=args.end_scale,
            step=args.step,
            shift_step=args.shift_step,
            warmup_duration=args.warmup_duration,
            warmup_scale=args.warmup_scale,
            min_divisor=args.min_divisor,
            max_multiplier=args.max_multiplier,
            name_suffix=name_suffix,
            dry_run=args.dry_run,
        )

        builder = SweepTraceBuilder(config)
        invocations_df, durations_df = builder.run()

        if args.dry_run:
            print("\n[INFO] Dry-run enabled; skipping output file generation.")
            with pd.option_context("display.max_columns", None, "display.width", 1200):
                print("\n--- Generated Invocations (first 3 rows per workload) ---")
                groups = [g.head(3) for _, g in invocations_df.groupby("FunctionName", sort=False)]
                print(pd.concat(groups))
                print("\n--- Generated Durations ---")
                print(durations_df)
            return 0

        # Write output files
        output_dir = Path(args.output)
        output_dir.mkdir(parents=True, exist_ok=True)
        invocations_path = output_dir / "invocations.csv"
        durations_path = output_dir / "durations.csv"

        invocations_df.to_csv(invocations_path, index=False)
        durations_df.to_csv(durations_path, index=False)

        print(f"\n[SUCCESS] Wrote sweep traces to: {output_dir.resolve()}")
        print(f"  - {invocations_path.name}: {len(invocations_df)} rows, "
              f"{len(invocations_df.columns) - 1} time columns")
        print(f"  - {durations_path.name}: {len(durations_df)} workloads")

    except Exception as e:
        print(f"[ERROR] {e}", file=sys.stderr)
        import traceback
        traceback.print_exc()
        return 1

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
