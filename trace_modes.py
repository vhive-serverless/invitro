"""Side-effect-free experiment mode and workload-name mapping."""

MODE_INVM_PY = "invm-py"
MODE_NEXUS_GO = "nexus-go"
MODE_NEXUS_RDMA = "nexus-rdma"
TRACE_MODES = (MODE_INVM_PY, MODE_NEXUS_GO, MODE_NEXUS_RDMA)

MATCHED_WORKLOADS = ("pyaesserve", "mapper", "reducer")
GO_WORKLOAD_NAMES = {
    "pyaesserve": "gopyaesserve",
    "mapper": "gomapper",
    "reducer": "goreducer",
}


def trace_workload_name(canonical_name: str, mode: str) -> str:
    """Return the snapshot identity selected by an explicit experiment mode."""
    if canonical_name not in MATCHED_WORKLOADS:
        raise ValueError(
            f"workload {canonical_name!r} is not in the matched Python/Go subset: "
            f"{', '.join(MATCHED_WORKLOADS)}"
        )
    if mode == MODE_INVM_PY:
        return canonical_name
    go_name = GO_WORKLOAD_NAMES[canonical_name]
    if mode == MODE_NEXUS_GO:
        return f"{go_name}-s3-rpc-stream"
    if mode == MODE_NEXUS_RDMA:
        # RDMA is selected by backend metadata. Khala's existing snapshot
        # grammar deliberately has no "rdma" token.
        return f"{go_name}-s3-rpc"
    raise ValueError(f"unsupported experiment mode: {mode}")


def canonical_workload_name(trace_name: str) -> str:
    """Recover the canonical identity before looking up duration/RPS data."""
    stem = trace_name.split("-", 1)[0]
    for canonical, go_name in GO_WORKLOAD_NAMES.items():
        if stem == go_name:
            return canonical
    return stem
