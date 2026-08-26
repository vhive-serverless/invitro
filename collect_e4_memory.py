#!/usr/bin/env python3
"""Co-sample Firecracker PSS and worker memory during an E3/E4 cell."""
from __future__ import annotations

import argparse
import csv
import json
import shlex
import subprocess
import time
from concurrent.futures import ThreadPoolExecutor
from datetime import datetime, timezone
from pathlib import Path


REMOTE_PROGRAM = r'''
import glob,json,os
targets={"firecracker","nexus-backend"}
processes=[]
for proc in glob.glob("/proc/[0-9]*"):
    try:
        pid=int(proc.rsplit("/",1)[1])
        with open(proc+"/comm",encoding="utf-8") as h: name=h.read().strip()
        if name not in targets: continue
        pss=None
        with open(proc+"/smaps_rollup",encoding="utf-8") as h:
            for line in h:
                if line.startswith("Pss:"):
                    pss=int(line.split()[1]); break
        if pss is not None: processes.append({"name":name,"pid":pid,"pss_kib":pss})
    except (FileNotFoundError,PermissionError,ProcessLookupError,ValueError): pass
mem={}
with open("/proc/meminfo",encoding="utf-8") as h:
    for line in h:
        key,value,*_=line.replace(":"," ").split()
        if key in ("MemTotal","MemAvailable"): mem[key]=int(value)
print(json.dumps({"processes":processes,"memtotal_kib":mem.get("MemTotal"),"memavailable_kib":mem.get("MemAvailable")}))
'''.strip()

SAMPLE_FIELDS = (
    "timestamp_utc", "mode", "repetition", "worker", "firecracker_processes",
    "firecracker_total_pss_kib", "firecracker_mean_pss_kib", "worker_memtotal_kib",
    "worker_memavailable_kib", "worker_used_kib", "status",
)
BACKEND_FIELDS = (
    "timestamp_utc", "mode", "repetition", "worker", "backend_processes",
    "backend_total_pss_kib", "status",
)


def summarize(payload: dict, worker: str, mode: str, repetition: int, timestamp: str) -> tuple[dict, dict]:
    processes = payload.get("processes")
    if not isinstance(processes, list):
        raise ValueError("remote payload lacks process list")
    firecracker = [int(item["pss_kib"]) for item in processes if item.get("name") == "firecracker"]
    backend = [int(item["pss_kib"]) for item in processes if item.get("name") == "nexus-backend"]
    total = payload.get("memtotal_kib")
    available = payload.get("memavailable_kib")
    if not isinstance(total, int) or not isinstance(available, int) or total < available:
        raise ValueError("remote payload has invalid worker memory")
    sample = {
        "timestamp_utc": timestamp,
        "mode": mode,
        "repetition": repetition,
        "worker": worker,
        "firecracker_processes": len(firecracker),
        "firecracker_total_pss_kib": sum(firecracker),
        "firecracker_mean_pss_kib": sum(firecracker) / len(firecracker) if firecracker else 0,
        "worker_memtotal_kib": total,
        "worker_memavailable_kib": available,
        "worker_used_kib": total - available,
        "status": "ok",
    }
    backend_row = {
        "timestamp_utc": timestamp,
        "mode": mode,
        "repetition": repetition,
        "worker": worker,
        "backend_processes": len(backend),
        "backend_total_pss_kib": sum(backend),
        "status": "ok",
    }
    return sample, backend_row


def sample_worker(worker: str, mode: str, repetition: int) -> tuple[dict, dict]:
    timestamp = datetime.now(timezone.utc).isoformat()
    command = "sudo -n python3 -c " + shlex.quote(REMOTE_PROGRAM)
    completed = subprocess.run(
        ["ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=5", worker, command],
        capture_output=True,
        text=True,
        timeout=10,
        check=False,
    )
    if completed.returncode != 0:
        detail = completed.stderr.strip().replace("\n", " ")[:240]
        sample = {field: "" for field in SAMPLE_FIELDS}
        sample.update(timestamp_utc=timestamp, mode=mode, repetition=repetition, worker=worker,
                      status=f"error:{completed.returncode}:{detail}")
        backend = {field: "" for field in BACKEND_FIELDS}
        backend.update(timestamp_utc=timestamp, mode=mode, repetition=repetition, worker=worker,
                       status=f"error:{completed.returncode}:{detail}")
        return sample, backend
    try:
        return summarize(json.loads(completed.stdout), worker, mode, repetition, timestamp)
    except (json.JSONDecodeError, TypeError, ValueError, KeyError) as error:
        sample = {field: "" for field in SAMPLE_FIELDS}
        sample.update(timestamp_utc=timestamp, mode=mode, repetition=repetition, worker=worker,
                      status=f"error:parse:{error}")
        backend = {field: "" for field in BACKEND_FIELDS}
        backend.update(timestamp_utc=timestamp, mode=mode, repetition=repetition, worker=worker,
                       status=f"error:parse:{error}")
        return sample, backend


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--workers", required=True, help="Comma-separated worker IPs or SSH hosts")
    parser.add_argument("--mode", required=True)
    parser.add_argument("--repetition", required=True, type=int)
    parser.add_argument("--interval-seconds", type=float, default=1.0)
    parser.add_argument("--stop-file", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    parser.add_argument("--backend-output", required=True, type=Path)
    args = parser.parse_args()
    workers = tuple(item.strip() for item in args.workers.split(",") if item.strip())
    if not workers or args.interval_seconds <= 0:
        parser.error("workers and a positive interval are required")
    args.output.parent.mkdir(parents=True, exist_ok=True)
    backend_written: set[str] = set()
    with args.output.open("x", newline="", encoding="utf-8") as sample_handle, \
            args.backend_output.open("x", newline="", encoding="utf-8") as backend_handle:
        sample_writer = csv.DictWriter(sample_handle, fieldnames=SAMPLE_FIELDS)
        backend_writer = csv.DictWriter(backend_handle, fieldnames=BACKEND_FIELDS)
        sample_writer.writeheader()
        backend_writer.writeheader()
        while True:
            started = time.monotonic()
            with ThreadPoolExecutor(max_workers=len(workers)) as pool:
                rows = list(pool.map(lambda worker: sample_worker(worker, args.mode, args.repetition), workers))
            for sample, backend in rows:
                sample_writer.writerow(sample)
                if backend["worker"] not in backend_written:
                    backend_writer.writerow(backend)
                    backend_written.add(str(backend["worker"]))
            sample_handle.flush()
            backend_handle.flush()
            if args.stop_file.exists():
                break
            time.sleep(max(0.0, args.interval_seconds - (time.monotonic() - started)))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
