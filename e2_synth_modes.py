#!/usr/bin/env python3
"""Canonical E2-Synth mode and payload definitions.

This module is deliberately separate from trace_modes.py: changing the E2
synthetic matrix must not alter any existing E1/E2 workload contract.
"""

from __future__ import annotations

import re

MODES = (
    "invm-py", "invm-js", "invm-go", "hosttcp-go", "nexus-py",
    "nexus-js", "nexus-go", "nexus-rdma-py", "nexus-rdma-go",
)
PAYLOADS = (4, 4096, 16384, 65536, 262144, 1048576, 2097152,
            4194304, 8388608, 16777216)
MODE_ALIAS = {"nexus-rdma": "nexus-rdma-go"}
INVM_MODES = frozenset(("invm-py", "invm-js", "invm-go"))
RDMA_MODES = frozenset(("nexus-rdma-py", "nexus-rdma-go"))
_RFC1123 = re.compile(r"^[a-z0-9]([-a-z0-9]*[a-z0-9])?$")


def canonical_mode(mode: str) -> str:
    mode = MODE_ALIAS.get(mode.strip(), mode.strip())
    if mode not in MODES:
        raise ValueError(f"unsupported E2-Synth mode: {mode!r}")
    return mode


def workload_name(payload: int, mode: str) -> str:
    """Return the snapshot identity used by Khala's synthetic parser."""
    if payload not in PAYLOADS:
        raise ValueError(f"unsupported E2-Synth payload: {payload}")
    mode = canonical_mode(mode)
    base = f"synthetic_e_0_p_{payload}"
    if mode == "invm-py":
        return base
    if mode == "invm-go":
        return "go" + base
    if mode == "invm-js":
        return "js" + base
    language_base = base if mode.endswith("-py") else ("go" + base if mode.endswith("-go") else "js" + base)
    transport = "hosttcp" if mode == "hosttcp-go" else ("rdma" if mode in RDMA_MODES else "shmem")
    return f"{language_base}-s3-rpc-{transport}"


def trace_workload_name(payload: int, mode: str) -> str:
    """Return the RFC1123-safe service identity used in generated traces.

    Khala receives the underscore-form ``workload_name`` for deployment and
    snapshot creation.  Its snapshot parser maps this hyphenated service name
    back to the exact underscore-form VM workload before selecting a snapshot.
    """
    canonical = workload_name(payload, mode)
    service = canonical.replace("_", "-")
    if len(service) > 63 or _RFC1123.fullmatch(service) is None:
        raise ValueError(f"synthetic trace identity is not RFC1123-safe: {service!r}")
    return service


def attaches_shared_memory(mode: str) -> bool:
    return canonical_mode(mode) not in INVM_MODES
