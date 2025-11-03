#!/usr/bin/env python3
"""
Build scaled function invocation traces with configurable load scaling.

This script generates invocation traces by:
1. Dividing RPS by a divisor to get minimum load
2. Scaling the load from start-scale to end-scale with a given step
3. Adding a warmup phase with linear ramp-up
4. Optionally adding a max-scale phase at the end
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

DEFAULT_INPUT_RPS = {
    "chameleonserve": 1000, "cnnserve": 120, "imageresize": 24, "lrserving": 950,
    "mapper": 75, "pyaesserve": 1400, "reducer": 16, "rnnserve": 600,
    "streducer": 320, "sttrainer": 275,
}

DEFAULT_WORKLOAD_AVG_DURATION_MS = {
    "chameleonserve": 80.62, "cnnserve": 481.005, "imageresize": 2070.765,
    "lrserving": 106.3495, "mapper": 809.065, "pyaesserve": 55.638,
    "reducer": 4935.275, "rnnserve": 101.7505, "streducer": 312.2645,
    "sttrainer": 213.7305,
}

DEFAULT_OUTPUT_DIR = "data/traces/nexus"

# --- Configuration Class ---

@dataclass
class Config:
    """Configuration for the scaled trace builder."""
    workload_rps: Dict[str, float]
    output_dir: Path
    divisor: float = 10.0
    start_scale: float = 1.0
    end_scale: float = 10.0
    step: float = 1.0
    warmup_duration: int = 3
    warmup_scale: float = 1.0
    max_scale: float | None = None
    single_workload: str | None = None
    name_suffix: str = ""
    dry_run: bool = False

# --- Core Logic Class ---

class ScaledTraceBuilder:
    """Builds scaled invocation traces based on RPS and scaling configuration."""

    def __init__(self, config: Config):
        self.config = config

    def run(self) -> tuple[pd.DataFrame, pd.DataFrame]:
        """Execute the trace building pipeline."""
        print("[INFO] Building scaled trace...")
        
        # Calculate minimum load for each workload
        min_loads = self._calculate_min_loads()
        
        # Generate column names based on warmup and scaling
        column_names = self._generate_column_names()
        
        print(f"[INFO] Generated columns: {column_names[:5]}...{column_names[-5:]}")
        
        # Build the invocations dataframe
        invocations_df = self._build_invocations(min_loads, column_names)
        
        # Build the durations dataframe
        durations_df = self._build_durations()
        
        return invocations_df, durations_df

    def _calculate_min_loads(self) -> Dict[str, int]:
        """Calculate minimum load (RPM) for each workload by dividing RPS by divisor."""
        min_loads = {}
        
        # Filter workloads if single-workload is specified
        workloads_to_process = self.config.workload_rps
        if self.config.single_workload:
            if self.config.single_workload not in self.config.workload_rps:
                raise ValueError(f"Workload '{self.config.single_workload}' not found in workload_rps mapping")
            workloads_to_process = {self.config.single_workload: self.config.workload_rps[self.config.single_workload]}
            print(f"[INFO] Filtering to single workload: {self.config.single_workload}")
        
        for workload, rps in workloads_to_process.items():
            min_rpm = int((rps / self.config.divisor) * 60)
            min_loads[workload] = min_rpm
            print(f"[INFO] {workload}: RPS={rps}, Min Load (RPM)={min_rpm}")
        return min_loads

    def _generate_column_names(self) -> List[str]:
        """Generate column names based on warmup duration and scaling parameters."""
        columns = []
        
        # Warmup columns (negative indices)
        if self.config.warmup_duration > 0:
            warmup_cols = [str(-i) for i in range(self.config.warmup_duration, 0, -1)]
            columns.extend(warmup_cols)
        
        # Main scaling columns (positive indices)
        current_scale = self.config.start_scale
        col_idx = 1
        while current_scale <= self.config.end_scale:
            columns.append(str(col_idx))
            current_scale += self.config.step
            col_idx += 1
        
        # Max scale column (if specified)
        if self.config.max_scale is not None:
            columns.append(str(col_idx))
        
        return columns

    def _build_invocations(self, min_loads: Dict[str, int], columns: List[str]) -> pd.DataFrame:
        """Build the invocations dataframe with scaled load values."""
        rows = []
        
        for workload, min_load in min_loads.items():
            workload_name = f"{workload}{self.config.name_suffix}"
            row = {"FunctionName": workload_name}
            
            # Calculate how many warmup columns and main columns we have
            warmup_count = self.config.warmup_duration if self.config.warmup_duration > 0 else 0
            main_columns = [c for c in columns if int(c) > 0]
            
            # Fill warmup columns with linear ramp-up to warmup_scale
            if warmup_count > 0:
                warmup_target = int(min_load * self.config.warmup_scale)
                for i, col in enumerate(columns[:warmup_count], start=1):
                    row[col] = int(warmup_target * i / warmup_count)
            
            # Fill main scaling columns
            current_scale = self.config.start_scale
            for i, col in enumerate(main_columns):
                if self.config.max_scale is not None and i == len(main_columns) - 1:
                    # Last column is max_scale
                    row[col] = int(min_load * self.config.max_scale)
                else:
                    row[col] = int(min_load * current_scale)
                    current_scale += self.config.step
            
            rows.append(row)
        
        # Create dataframe with columns in the correct order
        df = pd.DataFrame(rows)
        # Reorder columns to ensure FunctionName is first, followed by time columns
        ordered_columns = ["FunctionName"] + columns
        return df[ordered_columns]

    def _build_durations(self) -> pd.DataFrame:
        """Build the durations dataframe with average duration for each workload."""
        duration_rows = []
        
        # Use the same workload filtering logic as _calculate_min_loads
        workloads_to_process = self.config.workload_rps
        if self.config.single_workload:
            workloads_to_process = {self.config.single_workload: self.config.workload_rps[self.config.single_workload]}
        
        for workload in workloads_to_process.keys():
            workload_name = f"{workload}{self.config.name_suffix}"
            duration_ms = DEFAULT_WORKLOAD_AVG_DURATION_MS.get(workload, 100.0)
            duration_rows.append({
                "FunctionName": workload_name,
                "AvgDurationMs": duration_ms
            })
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
    parser = argparse.ArgumentParser(
        description="Build scaled function invocation traces with configurable load scaling.",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  # Basic usage with default RPS values
  %(prog)s --divisor 10 --start-scale 2 --end-scale 10 --step 2 --warmup-duration 3

  # With custom RPS values and max-scale
  %(prog)s --workload-rps '{"app1": 100, "app2": 200}' --divisor 5 --start-scale 1 --end-scale 5 --step 1 --max-scale 20

  # With suffix flags
  %(prog)s --divisor 10 --start-scale 2 --end-scale 10 --step 2 --s3 --rpc
        """
    )
    
    # Required arguments
    parser.add_argument("--divisor", type=float, required=True,
                        help="Divisor to calculate minimum load from RPS (required)")
    parser.add_argument("--start-scale", type=float, required=True,
                        help="Starting scale multiplier for minimum load (required)")
    parser.add_argument("--end-scale", type=float, required=True,
                        help="Ending scale multiplier for minimum load (required)")
    parser.add_argument("--step", type=float, required=True,
                        help="Step size for scale progression (required)")
    
    # Optional arguments
    parser.add_argument("--workload-rps", type=parse_workload_rps_arg, default=DEFAULT_INPUT_RPS,
                        help="Workload->RPS mapping as a JSON file path or inline JSON. Default: built-in values")
    parser.add_argument("--single-workload", type=str, default=None,
                        help="If specified, only generate trace for this single workload")
    parser.add_argument("--output", default=DEFAULT_OUTPUT_DIR,
                        help=f"Directory to write output CSVs. Default: {DEFAULT_OUTPUT_DIR}")
    parser.add_argument("--warmup-duration", type=int, default=0,
                        help="Duration of warmup phase in minutes (columns with negative indices). Default: 3")
    parser.add_argument("--warmup-scale", type=float, default=1.0,
                        help="Scale multiplier for warmup target (warmup ramps up to min_load * warmup_scale). Default: 1.0")
    parser.add_argument("--max-scale", type=float, default=None,
                        help="Optional maximum scale to add as the final column. If not specified, no max-scale column is added.")
    parser.add_argument("--s3", action="store_true",
                        help="Append '-s3' suffix to workload names")
    parser.add_argument("--rpc", action="store_true",
                        help="Append '-rpc' suffix to workload names")
    parser.add_argument("--dry-run", action="store_true",
                        help="Run all steps but do not write output files")
    
    return parser

def main(argv: List[str] | None = None) -> int:
    """Main execution function."""
    parser = build_arg_parser()
    args = parser.parse_args(list(argv) if argv is not None else None)

    # Validate scaling parameters
    if args.start_scale > args.end_scale:
        print("[ERROR] start-scale must be less than or equal to end-scale", file=sys.stderr)
        return 1
    
    if args.step <= 0:
        print("[ERROR] step must be greater than 0", file=sys.stderr)
        return 1
    
    if args.divisor <= 0:
        print("[ERROR] divisor must be greater than 0", file=sys.stderr)
        return 1

    # Build the name suffix based on flags, ensuring order: -s3 before -rpc
    suffix_parts = []
    if args.s3:
        suffix_parts.append("s3")
    if args.rpc:
        suffix_parts.append("rpc")
    name_suffix = f"-{'-'.join(suffix_parts)}" if suffix_parts else ""

    try:
        config = Config(
            workload_rps=args.workload_rps,
            output_dir=Path(args.output),
            divisor=args.divisor,
            start_scale=args.start_scale,
            end_scale=args.end_scale,
            step=args.step,
            warmup_duration=args.warmup_duration,
            warmup_scale=args.warmup_scale,
            max_scale=args.max_scale,
            single_workload=args.single_workload,
            name_suffix=name_suffix,
            dry_run=args.dry_run,
        )

        builder = ScaledTraceBuilder(config)
        invocations_df, durations_df = builder.run()

        if args.dry_run:
            print("\n[INFO] Dry-run enabled; skipping output file generation.")
            print("\n--- Generated Invocations ---")
            with pd.option_context('display.max_columns', None, 'display.width', 1000):
                print(invocations_df)
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
        
        print(f"\n[SUCCESS] Wrote scaled traces to: {output_dir.resolve()}")
        print(f"  - {invocations_path.name}: {len(invocations_df)} workloads, {len(invocations_df.columns)-1} time columns")
        print(f"  - {durations_path.name}: {len(durations_df)} workloads")

    except Exception as e:
        print(f"[ERROR] A problem occurred: {e}", file=sys.stderr)
        import traceback
        traceback.print_exc()
        return 1
        
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
