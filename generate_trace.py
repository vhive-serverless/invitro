#!/usr/bin/env python3
"""
Generate function invocation traces from a base reference CSV.

This script ports the logic from map_traces.ipynb into a reusable CLI.
The refactored version encapsulates the logic within a TraceGenerator class
for improved modularity and maintainability.
"""

from __future__ import annotations

import argparse
import json
import sys
from dataclasses import dataclass, field
from pathlib import Path
from typing import Dict, Iterable, List, Tuple

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
    "mapper": 60, "pyaesserve": 500, "reducer": 12, "rnnserve": 150, 
    "streducer": 160, "sttrainer": 130
}

DEFAULT_WORKLOAD_AVG_DURATION_MS: Dict[str, float] = {
    "chameleonserve": 44.22, "cnnserve": 585.589, "imageresize": 2113.782,
	"lrserving": 47.26, "mapper": 775.300, "pyaesserve": 28.542,
	"reducer": 4037.270, "rnnserve": 197.117, "streducer": 154.807,
	"sttrainer": 244.291,
}

# --- Configuration Class ---

@dataclass
class Config:
    """Configuration for the trace generator."""
    input_path: Path
    output_dir: Path
    mode: str = "synthetic"
    workload_rps: Dict[str, float] = field(default_factory=lambda: DEFAULT_WORKLOAD_RPS)
    function_multiplier: int = 1
    shift_step: int = 1
    selection_divisor: float = 10.0
    min_divisor: float = 10.0
    max_multiplier: float = 2.0
    name_suffix: str = ""
    warmup_minutes: int = 2
    dry_run: bool = False

# --- Core Logic Class ---

class TraceGenerator:
    """Generates invocation traces based on a reference dataset and configuration."""

    def __init__(self, config: Config):
        self.config = config
        self.base_df = self._load_base_trace()
        self.time_cols = self._get_time_columns(self.base_df)
        self.invocation_stats_df = self._compute_invocation_stats()

    def run(self) -> Tuple[pd.DataFrame, pd.DataFrame]:
        """Execute the full trace generation pipeline."""
        print("[INFO] Selecting representative functions per workload...")
        template_df = self._build_workload_templates()

        print("[INFO] Generating final trace with time-shifted duplicates...")
        generated_trace = self._generate_from_templates(template_df)
        
        if self.config.warmup_minutes > 0:
            print(f"[INFO] Adding a {self.config.warmup_minutes}-minute warmup phase...")
            generated_trace = self._add_warmup(generated_trace)

        print("[INFO] Creating average duration data...")
        duration_df = self._create_duration_df(generated_trace)
        
        return generated_trace, duration_df

    def _load_base_trace(self) -> pd.DataFrame:
        """Loads and pre-filters the base invocation trace CSV."""
        print(f"[INFO] Loading base invocation trace: {self.config.input_path}")
        df = pd.read_csv(self.config.input_path)
        df.columns = df.columns.map(str)
        # Filter to HTTP-triggered functions only, as in the original script
        return df[df["Trigger"] == "http"].copy()

    @staticmethod
    def _get_time_columns(df: pd.DataFrame) -> List[str]:
        """Return columns that look like numeric time indexes."""
        cols = [c for c in df.columns if isinstance(c, str) and c.isdigit()]
        if not cols:
            raise ValueError("No numbered time-series columns detected in base trace.")
        return cols

    def _compute_invocation_stats(self) -> pd.DataFrame:
        """Compute per-row stats over the time-series columns."""
        print("[INFO] Computing per-function statistics...")
        df = self.base_df
        time_series_data = df[self.time_cols]
        
        stats = pd.DataFrame(index=df.index)
        stats["invocation_count_sum"] = time_series_data.sum(axis=1)
        stats["invocation_count_avg"] = time_series_data.mean(axis=1)
        stats["invocation_count_max"] = time_series_data.max(axis=1)
        stats["invocation_count_min"] = time_series_data.min(axis=1)

        # Combine metadata, stats, and time-series data
        meta_cols = [c for c in ["HashOwner", "HashApp", "HashFunction", "Trigger"] if c in df.columns]
        result = pd.concat([df[meta_cols], stats, time_series_data], axis=1)
        return result.sort_values(by="invocation_count_sum", ascending=False)

    def _build_workload_templates(self) -> pd.DataFrame:
        """Construct a template trace by selecting a representative function for each workload."""
        template_rows = []
        for workload, target_rps in self.config.workload_rps.items():
            # The original logic scales the target RPS for selection
            selection_rps = target_rps / self.config.selection_divisor
            
            if self.config.mode == "trace":
                selected_fn = self._select_closest_function(selection_rps)
            elif self.config.mode == "synthetic":
                selected_fn = self._create_synthetic_function(selection_rps)
            else:
                raise ValueError(f"Unknown mode: {self.config.mode}")

            # Add FunctionName and target_rps columns for parity with original script
            workload_name = f"{workload}{self.config.name_suffix}"
            selected_fn["FunctionName"] = workload_name
            selected_fn["target_rps"] = selection_rps * 60
            template_rows.append(selected_fn)

        return pd.DataFrame(template_rows).reset_index(drop=True)

    def _select_closest_function(self, target_rps: float) -> pd.Series:
        """Filter and select the function whose average invocation rate is closest to the target."""
        target_rpm = target_rps * 60.0
        min_rpm_bound = target_rpm / self.config.min_divisor
        max_rpm_bound = target_rpm * self.config.max_multiplier
        
        candidates = self.invocation_stats_df[
            (self.invocation_stats_df["invocation_count_max"] < max_rpm_bound) &
            (self.invocation_stats_df["invocation_count_min"] > min_rpm_bound)
        ]

        if candidates.empty:
            raise ValueError(f"No function found for target_rps {target_rps}")

        # Find the row with the minimum absolute difference from the target RPM
        diff = (candidates["invocation_count_avg"] - target_rpm).abs()
        best_match_idx = diff.idxmin()
        return candidates.loc[best_match_idx].copy()

    def _create_synthetic_function(self, target_rps: float) -> pd.Series:
        """Create a function trace with a constant invocation rate."""
        target_rpm = int(target_rps * 60.0)
        # Create a new Series with the same structure as a row from invocation_stats_df
        synthetic_row = pd.Series(index=self.invocation_stats_df.columns, dtype='object')
        # Fill time columns with the constant target RPM
        synthetic_row[self.time_cols] = target_rpm
        return synthetic_row

    def _generate_from_templates(self, template_df: pd.DataFrame) -> pd.DataFrame:
        """Duplicate and time-shift/scale each workload's template function."""
        generated_rows = []
        for _, template_row in template_df.iterrows():
            if self.config.mode == "trace":
                generated_rows.extend(self._generate_shifted_rows(template_row))
            elif self.config.mode == "synthetic":
                generated_rows.extend(self._generate_scaled_rows(template_row))
        
        # Keep only FunctionName and time-series columns
        final_cols = ["FunctionName"] + self.time_cols
        return pd.DataFrame(generated_rows)[final_cols]

    def _generate_shifted_rows(self, template_row: pd.Series) -> List[Dict]:
        """Generate multiple rows by time-shifting (rotating) the time-series data."""
        rows = []
        values = template_row[self.time_cols].tolist()
        trace_length = len(values)

        for i in range(self.config.function_multiplier):
            new_row = template_row.copy()
            if i > 0 and trace_length > 0:
                shift = (self.config.shift_step * i) % trace_length
                shifted_values = values[shift:] + values[:shift]
                new_row[self.time_cols] = shifted_values
            rows.append(new_row.to_dict())
        return rows

    def _generate_scaled_rows(self, template_row: pd.Series) -> List[Dict]:
        """Generate a single row by scaling the RPM by the function multiplier."""
        new_row = template_row.copy()
        scaled_values = [v * self.config.function_multiplier for v in new_row[self.time_cols]]
        new_row[self.time_cols] = scaled_values
        return [new_row.to_dict()] # Return as a list of one

    def _add_warmup(self, df: pd.DataFrame) -> pd.DataFrame:
        """Prepend warmup columns that linearly ramp up to the first minute's value."""
        warmup_minutes = self.config.warmup_minutes
        first_col = self.time_cols[0]
        
        # Create new column names for the warmup phase (e.g., -2, -1)
        warmup_cols = [str(i) for i in range(-warmup_minutes, 0)]
        warmup_df = pd.DataFrame(index=df.index, columns=warmup_cols, dtype=int)
        
        for idx, row in df.iterrows():
            first_val = float(row[first_col])
            for k, col_name in enumerate(warmup_cols, start=1):
                warmup_df.at[idx, col_name] = int(first_val * k / warmup_minutes)
        
        warmup_df = warmup_df.astype(int)
        # Concatenate: FunctionName | warmup columns | original time columns
        return pd.concat([df[["FunctionName"]], warmup_df, df[self.time_cols]], axis=1)

    @staticmethod
    def _create_duration_df(final_trace_df: pd.DataFrame) -> pd.DataFrame:
        """Create a DataFrame mapping each function to its average duration."""
        duration_rows = []
        for _, row in final_trace_df.iterrows():
            func_name = row['FunctionName']
            # Remove suffixes like -s3 or -rpc to find the base workload name
            base_workload = func_name.split('-')[0]
            duration_ms = DEFAULT_WORKLOAD_AVG_DURATION_MS.get(base_workload, 100.0)
            duration_rows.append({"FunctionName": func_name, "AvgDurationMs": duration_ms})
        return pd.DataFrame(duration_rows)


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
    parser = argparse.ArgumentParser(description="Generate function invocation traces from a reference CSV.")
    parser.add_argument("--mode", choices=["synthetic", "trace"], default="synthetic", help="Mode: 'synthetic' for constant load, 'trace' for pattern-based.")
    parser.add_argument("--input", default=DEFAULT_INPUT, help=f"Path to base invocation CSV. Default: {DEFAULT_INPUT}")
    parser.add_argument("--output", default=DEFAULT_OUTPUT_DIR, help=f"Directory to write output CSVs. Default: {DEFAULT_OUTPUT_DIR}")
    parser.add_argument("--workload-rps", dest="workload_rps", type=parse_workload_rps_arg, default=DEFAULT_WORKLOAD_RPS, help="Workload->RPS mapping as a JSON file path or inline JSON.")
    parser.add_argument("--function-multiplier", type=int, default=1, help="Functions per workload to generate. In 'trace' mode, creates time-shifted duplicates. In 'synthetic' mode, scales the load.")
    parser.add_argument("--shift-step", type=int, default=1, help="Shift step for duplicates in 'trace' mode.")
    parser.add_argument("--selection-divisor", type=float, default=10.0, help="Divide target RPS by this when selecting functions.")
    parser.add_argument("--min-divisor", type=float, default=10.0, help="Lower bound filter for function selection.")
    parser.add_argument("--max-multiplier", type=float, default=2.0, help="Upper bound filter for function selection.")
    parser.add_argument("--s3", action="store_true", help="Append '-s3' suffix to workload names.")
    parser.add_argument("--rpc", action="store_true", help="Append '-rpc' suffix to workload names.")
    parser.add_argument("--warmup", type=int, default=2, help="Warmup duration in minutes to prepend to the trace.")
    parser.add_argument("--dry-run", action="store_true", help="Run all steps but do not write output files.")
    return parser

def main(argv: Iterable[str] | None = None) -> int:
    """Main execution function."""
    parser = build_arg_parser()
    args = parser.parse_args(list(argv) if argv is not None else None)

    if not Path(args.input).exists():
        print(f"[ERROR] Input file not found: {args.input}", file=sys.stderr)
        return 1
        
    # Build the name suffix based on flags, ensuring order: -s3 before -rpc
    suffix_parts = []
    if args.s3: suffix_parts.append("s3")
    if args.rpc: suffix_parts.append("rpc")
    name_suffix = f"-{'-'.join(suffix_parts)}" if suffix_parts else ""

    try:
        config = Config(
            input_path=Path(args.input),
            output_dir=Path(args.output),
            mode=args.mode,
            workload_rps=args.workload_rps,
            function_multiplier=args.function_multiplier,
            shift_step=args.shift_step,
            selection_divisor=args.selection_divisor,
            min_divisor=args.min_divisor,
            max_multiplier=args.max_multiplier,
            name_suffix=name_suffix,
            warmup_minutes=args.warmup,
            dry_run=args.dry_run,
        )

        generator = TraceGenerator(config)
        invocations_df, durations_df = generator.run()

        if args.dry_run:
            print("[INFO] Dry-run enabled; skipping output file generation.")
            with pd.option_context('display.max_columns', None, 'display.width', 1000):
                print("\n--- Generated Invocations (Head) ---")
                print(invocations_df.head())
                print("\n--- Generated Durations (Head) ---")
                print(durations_df.head())
            return 0

        # Write output files
        output_dir = Path(args.output)
        output_dir.mkdir(parents=True, exist_ok=True)
        invocations_path = output_dir / "invocations.csv"
        durations_path = output_dir / "durations.csv"
        
        invocations_df.to_csv(invocations_path, index=False)
        durations_df.to_csv(durations_path, index=False)
        
        print(f"\n[SUCCESS] Wrote generated traces to: {output_dir.resolve()}")

    except (ValueError, FileNotFoundError) as e:
        print(f"[ERROR] A problem occurred: {e}", file=sys.stderr)
        return 1
        
    return 0


if __name__ == "__main__":
    raise SystemExit(main())