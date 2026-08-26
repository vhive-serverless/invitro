"""Side-effect-free experiment mode and workload-name mapping."""

MODE_INVM_PY = "invm-py"
MODE_NEXUS_PY = "nexus-py"
MODE_NEXUS_GO = "nexus-go"
MODE_NEXUS_RDMA = "nexus-rdma"
MODE_NEXUS_RDMA_PY = "nexus-rdma-py"
MODE_HOSTTCP_GO = "hosttcp-go"
MODE_HOSTTCP_PY = "hosttcp-py"
# hosttcp-py is intentionally opt-in: its raw Python admission is terminally
# inadmissible, although the generator accepts its mode identity for functional
# follow-up work. The end-to-end matrix gates it separately in the shell plan.
TRACE_MODES = (
    MODE_INVM_PY,
    MODE_NEXUS_PY,
    MODE_NEXUS_GO,
    MODE_NEXUS_RDMA,
    MODE_NEXUS_RDMA_PY,
    MODE_HOSTTCP_GO,
    MODE_HOSTTCP_PY,
)

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
    if mode == MODE_NEXUS_PY:
        return f"{canonical_name}-s3-rpc-shmem"
    go_name = GO_WORKLOAD_NAMES[canonical_name]
    if mode == MODE_NEXUS_GO:
        return f"{go_name}-s3-rpc-shmem"
    if mode == MODE_NEXUS_RDMA:
        return f"{go_name}-s3-rpc-rdma"
    if mode == MODE_NEXUS_RDMA_PY:
        return f"{canonical_name}-s3-rpc-rdma"
    if mode == MODE_HOSTTCP_GO:
        return f"{go_name}-s3-rpc-hosttcp"
    if mode == MODE_HOSTTCP_PY:
        return f"{canonical_name}-s3-rpc-hosttcp"
    raise ValueError(f"unsupported experiment mode: {mode}")


def canonical_workload_name(trace_name: str) -> str:
    """Recover the canonical identity before looking up duration/RPS data."""
    stem = trace_name.split("-", 1)[0]
    for canonical, go_name in GO_WORKLOAD_NAMES.items():
        if stem == go_name:
            return canonical
    return stem
