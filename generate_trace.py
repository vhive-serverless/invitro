#!/usr/bin/env python3
"""
Generate function invocation traces from a base reference CSV.

This script ports the logic from map_traces.ipynb into a reusable CLI.

High-level steps:
1) Load the base invocation trace CSV (reference dataset).
2) Compute per-function statistics over the time-series columns.
3) For each workload and target RPS, select a representative function whose
   average invocation rate is closest to the target, while staying within a
   min/max bound derived from the target.
4) Generate the final trace by duplicating and time-shifting functions
   (wrap-around) per the requested multiplier.

Output: CSV with columns [FunctionName] + [minute columns]

Example:
  python generate_trace.py \
    --input data/traces/reference/preprocessed_150/invocations.csv \
    --output data/traces/nexus/invocations.csv \
    --function-multiplier 2

Optionally customize workload->RPS mapping via --workload-rps-json
  JSON example: {"mapper": 75, "reducer": 16}
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path
from typing import Dict, Iterable, List, Tuple

import pandas as pd


DEFAULT_INPUT = "data/traces/reference/preprocessed_150/invocations.csv"
DEFAULT_OUTPUT = "data/traces/nexus"


DEFAULT_WORKLOAD_RPS: Dict[str, float] = {
    "chameleonserve": 1000,
    "cnnserve": 120,
    "imageresize": 24,
    "lrserving": 950,
    "mapper": 75,
    "pyaesserve": 1400,
    "reducer": 16,
    "rnnserve": 600,
    "streducer": 320,
    "sttrainer": 275,
}

DEFAULT_WORKLOAD_AVG_DURATION_MS: Dict[str, float] = {
    "chameleonserve": 28.686,
    "cnnserve": 482.07,
    "imageresize": 2121.732,
    "lrserving": 88.197,
    "mapper": 816.582,
    "pyaesserve": 23.477,
    "reducer": 5143.899,
    "rnnserve": 80.3555,
    "streducer": 296.879,
    "sttrainer": 202.207,
}


def _get_numbered_columns(df: pd.DataFrame) -> List[str]:
    """Return columns that look like numeric time indexes (all digits or ints).

    Preserves the original column labels (could be int or str). A column
    qualifies if it's an int or a string comprised only of digits.
    """
    cols: List[str] = []
    for c in df.columns:
        if isinstance(c, str) and c.isdigit():
            cols.append(c)
    return cols


def compute_invocation_stats(base_invocation_trace: pd.DataFrame) -> Tuple[pd.DataFrame, List[str]]:
    """Compute per-row stats and return the augmented DataFrame and numbered columns.

    The input is expected to contain metadata columns such as HashOwner, HashApp,
    HashFunction, Trigger, followed by per-minute integer columns.
    """
    # Standardize all column labels to strings for consistent selection
    df = base_invocation_trace.copy()
    df = df[df["Trigger"] == "http"]  # Filter to HTTP-triggered functions only
    df.columns = df.columns.map(str)
    numbered_columns = _get_numbered_columns(df)
    
    # Keep original metadata columns if present
    meta_cols = [c for c in ["HashOwner", "HashApp", "HashFunction", "Trigger"] if c in df.columns]
    invocation = pd.DataFrame()
    if meta_cols:
        invocation = pd.concat([df[meta_cols], invocation], axis=1)

    invocation_count_only = df[numbered_columns]

    # Stats across the numbered columns
    invocation_count_only_stats = pd.DataFrame()
    # Use pandas operations directly instead of numpy
    invocation_count_only_stats["invocation_count_sum"] = invocation_count_only.sum(axis=1)
    invocation_count_only_stats["invocation_count_avg"] = invocation_count_only.mean(axis=1)
    invocation_count_only_stats["invocation_count_max"] = invocation_count_only.max(axis=1)
    invocation_count_only_stats["invocation_count_min"] = invocation_count_only.min(axis=1)

    invocation = pd.concat([invocation, invocation_count_only_stats], axis=1)
    invocation = pd.concat([invocation, invocation_count_only], axis=1)
    invocation = invocation.sort_values(by=["invocation_count_sum"], ascending=False)

    return invocation, numbered_columns


def get_function_per_target_rps(
    invocation: pd.DataFrame,
    target_rps: float,
    mode : str = "synthetic",
    *,
    min_divisor: float = 10.0,
    max_multiplier: float = 2.0,
) -> pd.DataFrame:
    """Filter and select the function closest to the target RPS.

    - We constrain each candidate's per-minute min and max counts using
      bounds derived from the target.
    - Then we pick the row whose average is closest to target RPM (RPS*60).
    - if mode is "synthetic", we directly use the target RPM and fill a new dataframe with that value
    - And we create a new DataFrame with just that row.

    Raises ValueError if no function matches the constraints.
    """
    if mode == "trace":
        target_invocation_min = target_rps * 60.0 / min_divisor
        target_invocation_max = target_rps * 60.0 * max_multiplier

        filt = invocation[
            (invocation["invocation_count_max"] < target_invocation_max)
            & (invocation["invocation_count_min"] > target_invocation_min)
        ].copy()

        if len(filt) == 0:
            raise ValueError(f"No function found for target_rps {target_rps}")

        target_rpm = target_rps * 60.0
        filt["invocation_count_avg_diff"] = (filt["invocation_count_avg"] - target_rpm).abs()
        # Pick the index with minimal difference to avoid sort typing issues
        best_idx = filt["invocation_count_avg_diff"].idxmin()
        return filt.loc[[best_idx]]
    elif mode == "synthetic":
        # In synthetic mode, we directly use the target_rpm, and not find 
        target_rpm = target_rps * 60.0
        # create a dataframe filled with target_rpm, and same length as invocation
        target_rpm_list = [target_rpm] * (len(invocation.columns)-8)
        target_rpm_list = [int(x) for x in target_rpm_list]
        invocation_df = pd.DataFrame([list(invocation.iloc[0, 0:8]) + target_rpm_list], index=[0], columns=invocation.columns)
        return invocation_df
    else:
        raise ValueError(f"Unknown mode: {mode}")


def build_per_workload_trace(
    invocation: pd.DataFrame,
    workload_rps: Dict[str, float],
    mode : str = "synthetic",
    *,
    selection_divisor: float = 10.0,
    min_divisor: float = 10.0,
    max_multiplier: float = 2.0,
    name_suffix: str = "",
) -> pd.DataFrame:
    """Construct a per-workload trace by selecting representative functions.

    selection_divisor controls how we reduce the given target RPS when searching
    for a representative function (defaults to 10, as in the notebook).
    The resulting DataFrame includes FunctionName and a 'target_rps' column
    (scaled by target_rps_scale) along with all original columns from the
    selected rows (stats + time series). The consumer may later trim columns.
    """
    per_workload_trace = pd.DataFrame()
    for workload, target_rps in workload_rps.items():
        # Notebook used target_rps / 10 when selecting
        selected = get_function_per_target_rps(
            invocation,
            target_rps / selection_divisor,
            mode = mode,
            min_divisor=min_divisor,
            max_multiplier=max_multiplier,
        )

        row = selected.copy()
        # Apply suffix to workload name if provided (e.g., -s3, -rpc, -s3-rpc)
        workload_name = f"{str(workload)}{name_suffix}"
        row.insert(0, "FunctionName", workload_name)
        # Notebook inserted target_rps multiplied by 6 (kept for parity)
        row.insert(1, "target_rps", target_rps / selection_divisor * 60)
        per_workload_trace = pd.concat([per_workload_trace, row], ignore_index=True)

    return per_workload_trace


def generate_trace(
    per_workload_trace: pd.DataFrame,
    function_multiplier: int,
    *,
    mode : str = "synthetic",
    shift_step: int = 1,
) -> Tuple[pd.DataFrame, pd.DataFrame]:
    """Duplicate and time-shift each workload's selected function.

    For each input row, create `function_multiplier` rows.
    The i-th duplicate is rotated left by (shift_step * i) positions across the
    time-series (numbered) columns, with wrap-around.
    """
    generated_trace = pd.DataFrame()
    # Ensure string columns for consistent indexing
    df = per_workload_trace.copy()
    df.columns = df.columns.map(str)
    numbered_columns = _get_numbered_columns(df)
    if not numbered_columns:
        raise ValueError("No numbered time-series columns detected in per_workload_trace")
    trace_length = len(numbered_columns)

    if mode == "trace":
        for _, row in df.iterrows():
            for i in range(function_multiplier):
                # create new function row
                new_row = row.copy()
                # Ensure FunctionName is a string label, stable across duplicates
                new_row["FunctionName"] = f"{row['FunctionName']}"

                if i > 0 and trace_length > 0:
                    shift_amount = (shift_step * i) % trace_length
                    values = new_row.loc[numbered_columns].tolist()
                    shifted = values[shift_amount:] + values[:shift_amount]
                    # Assign back in one shot
                    new_row.loc[numbered_columns] = shifted

                generated_trace = pd.concat([generated_trace, pd.DataFrame([new_row])], ignore_index=True)

    elif mode == "synthetic":
        for _, row in df.iterrows():
            # we multiply RPS with function multiplier
            new_row = row.copy()
            # Ensure FunctionName is a string label, stable across duplicates
            new_row["FunctionName"] = f"{row['FunctionName']}"
            
            values = row.loc[numbered_columns].tolist()
            scaled_values = [v * function_multiplier for v in values]
            new_row.loc[numbered_columns] = scaled_values
        
            generated_trace = pd.concat([generated_trace, pd.DataFrame([new_row])], ignore_index=True)
            
    # Keep only FunctionName and time-series columns
    # Ensure a DataFrame slice
    generated_trace = generated_trace.loc[:, ["FunctionName"] + numbered_columns]
    
    
    # add duration dataframe
    duration_df = pd.DataFrame()
    for _, row in generated_trace.iterrows():
        workload_name = row['FunctionName']
        # remove suffixes to get base workload name
        base_workload_name = workload_name.split('-')[0]
        duration_ms = DEFAULT_WORKLOAD_AVG_DURATION_MS.get(base_workload_name, 100.0)  # default to 100ms if not found
        duration_row = pd.DataFrame([[workload_name] + [duration_ms]], columns=["FunctionName", "AvgDurationMs"])
        duration_df = pd.concat([duration_df, duration_row], ignore_index=True)
    
    return generated_trace, duration_df


def parse_workload_rps_arg(value: str) -> Dict[str, float]:
    """Parse workload RPS mapping from a string.

    Accepts either a path to a JSON file or an inline JSON object.
    """
    p = Path(value)
    try:
        if p.exists():
            with p.open("r", encoding="utf-8") as f:
                return json.load(f)
        # Not a file: try to parse as JSON literal
        return json.loads(value)
    except Exception as e:
        raise argparse.ArgumentTypeError(f"Invalid workload RPS mapping: {e}")


def add_warmup_phase(df: pd.DataFrame, warmup: int) -> pd.DataFrame:
    """Add a warmup phase that linearly ramps up to the first minute's value.

    For each function row, we prepend `warmup` minute columns named (-warmup .. -1)
    whose values increase linearly from 0 (exclusive) up to the value of the
    first original time-series column (inclusive). The final warmup column (-1)
    matches the first real minute's invocation count.

    Example (warmup=3, first minute = 90):
      added cols: -3=30, -2=60, -1=90, then original columns start at 0,1,2,...
    """
    if warmup <= 0:
        return df

    df = df.copy()
    df.columns = df.columns.map(str)
    numbered_columns = _get_numbered_columns(df)
    if not numbered_columns:
        raise ValueError("No numbered time-series columns detected in the DataFrame")

    first_col = numbered_columns[0]

    # Warmup columns use negative labels so they are not re-detected later
    warmup_columns = [str(i) for i in range(-warmup, 0)]

    # Initialize with zeros (correct dtype)
    warmup_data = pd.DataFrame(
        0,
        index=df.index,
        columns=warmup_columns,
        dtype=df[first_col].dtype,
    )

    # Fill with linear ramp per row
    # Step k (1-based) value = first_val * k / warmup
    for idx in df.index:
        first_val = df.loc[idx, first_col]
        if pd.isna(first_val):
            continue
        # Convert to float to ensure numeric type for arithmetic
        first_val_numeric = float(first_val)
        for k, col in enumerate(warmup_columns, start=1):
            # if k == 1:
            #     warmup_data.at[idx, col] = min(20, int(first_val_numeric * 0.2 / warmup))
            # elif k == 2:
            #     warmup_data.at[idx, col] = int(first_val_numeric / warmup)
            # else:
            #     warmup_data.at[idx, col] = int(first_val_numeric * k / warmup)
            warmup_data.at[idx, col] = int(first_val_numeric * k / warmup)

    # Concatenate: FunctionName + warmup + original numbered columns
    df = pd.concat([df[["FunctionName"]], warmup_data, df[numbered_columns]], axis=1)
    return df

def build_arg_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="Generate function invocation traces from a reference CSV")
    parser.add_argument("--mode", choices=["synthetic", "trace"], default="synthetic", help="Operation mode (default: generate)")
    parser.add_argument("--input", default=DEFAULT_INPUT, help="Path to base invocation CSV (reference dataset)")
    parser.add_argument("--output", default=DEFAULT_OUTPUT, help="Path to write the generated trace CSV")
    parser.add_argument(
        "--workload-rps",
        dest="workload_rps",
        type=parse_workload_rps_arg,
        default=DEFAULT_WORKLOAD_RPS,
        help="Workload->RPS mapping as JSON path or inline JSON (default: built-in map)",
    )
    parser.add_argument("--function-multiplier", type=int, default=1, help="Number of functions per workload to generate (duplicates with time shifts)")
    parser.add_argument("--shift-step", type=int, default=1, help="Shift step per duplicate (positions to rotate left per increment)")
    parser.add_argument("--selection-divisor", type=float, default=10.0, help="Divide target RPS by this when selecting representative functions")
    parser.add_argument("--min-divisor", type=float, default=10.0, help="Lower bound: target_rps*60/min_divisor for per-minute minimum filter")
    parser.add_argument("--max-multiplier", type=float, default=2.0, help="Upper bound: target_rps*60*max_multiplier for per-minute maximum filter")
    # Suffix flags: append to workload names, ensuring -s3 precedes -rpc if both are set
    parser.add_argument("--s3", action="store_true", help="Append '-s3' to workload names")
    parser.add_argument("--rpc", action="store_true", help="Append '-rpc' to workload names (after '-s3' if both set)")
    parser.add_argument("--dry-run", action="store_true", help="Run selection and show summary without writing output")
    parser.add_argument("--warmup", type=int, default=2, help="Warmup duration in minutes to add to the trace")
    return parser


def main(argv: Iterable[str] | None = None) -> int:
    args = build_arg_parser().parse_args(list(argv) if argv is not None else None)

    input_path = Path(args.input)
    output_path = Path(args.output)

    if not input_path.exists():
        print(f"[ERROR] Input CSV not found: {input_path}", file=sys.stderr)
        return 2

    print(f"[INFO] Loading base invocation trace: {input_path}")
    base_df = pd.read_csv(input_path)

    print("[INFO] Computing per-function statistics...")
    invocation, numbered_columns = compute_invocation_stats(base_df)
    if not numbered_columns:
        print("[ERROR] No numbered time-series columns detected in base invocation trace.", file=sys.stderr)
        return 2

    print("[INFO] Selecting representative functions per workload...")
    try:
        # Build the name suffix based on flags, enforcing order: -s3 before -rpc
        suffix_parts: List[str] = []
        if args.s3:
            suffix_parts.append("s3")
        if args.rpc:
            suffix_parts.append("rpc")
        name_suffix = ("-" + "-".join(suffix_parts)) if suffix_parts else ""

        per_workload_trace = build_per_workload_trace(
            invocation,
            args.workload_rps,
            mode = args.mode,
            selection_divisor=args.selection_divisor,
            min_divisor=args.min_divisor,
            max_multiplier=args.max_multiplier,
            name_suffix=name_suffix,
        )
    except ValueError as e:
        print(f"[ERROR] {e}", file=sys.stderr)
        return 3

    print("[INFO] Generating final trace with time-shifted duplicates...")
    try:
        generated_trace, generated_duration = generate_trace(
            per_workload_trace,
            args.function_multiplier,
            mode = args.mode,
            shift_step=args.shift_step,
        )
    except ValueError as e:
        print(f"[ERROR] {e}", file=sys.stderr)
        return 3

    if args.dry_run:
        with pd.option_context('display.max_columns', None):
            print(generated_trace.head())
            print(generated_duration.head())
        print("[INFO] Dry-run enabled; not writing output.")
        return 0
    
    # add warmup phase
    generated_trace = add_warmup_phase(generated_trace, args.warmup)    

    output_path.mkdir(parents=True, exist_ok=True)
    generated_trace.to_csv(output_path / "invocations.csv", index=False)
    generated_duration.to_csv(output_path / "durations.csv", index=False)
    print(f"[INFO] Wrote generated trace to: {output_path}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
