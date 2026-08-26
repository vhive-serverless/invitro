"""Side-effect-free experiment mode and workload-name mapping."""

MODE_INVM_PY = "invm-py"
MODE_INVM_GO = "invm-go"
MODE_INVM_JS = "invm-js"
MODE_NEXUS_PY = "nexus-py"
MODE_NEXUS_GO = "nexus-go"
MODE_NEXUS_JS = "nexus-js"
MODE_NEXUS_RDMA = "nexus-rdma"
MODE_NEXUS_RDMA_PY = "nexus-rdma-py"
MODE_HOSTTCP_GO = "hosttcp-go"
MODE_HOSTTCP_PY = "hosttcp-py"
# hosttcp-py is intentionally opt-in: its raw Python admission is terminally
# inadmissible, although the generator accepts its mode identity for functional
# follow-up work. The end-to-end matrix gates it separately in the shell plan.
TRACE_MODES = (
    MODE_INVM_PY,
    MODE_INVM_GO,
    MODE_INVM_JS,
    MODE_NEXUS_PY,
    MODE_NEXUS_GO,
    MODE_NEXUS_JS,
    MODE_NEXUS_RDMA,
    MODE_NEXUS_RDMA_PY,
    MODE_HOSTTCP_GO,
    MODE_HOSTTCP_PY,
)

PYTHON_WORKLOADS = (
    "helloworld", "chameleonserve", "cnnserve", "imageresize", "lrserving",
    "mapper", "pyaesserve", "reducer", "rnnserve", "streducer", "sttrainer",
)
DENSITY_WORKLOADS = tuple(name for name in PYTHON_WORKLOADS if name != "helloworld")
# Kept as a compatibility name for scripts that mean the full Python E1 set.
MATCHED_WORKLOADS = PYTHON_WORKLOADS
GO_WORKLOAD_NAMES = {
    "helloworld": "gohelloworld",
    "pyaesserve": "gopyaesserve",
    "mapper": "gomapper",
    "reducer": "goreducer",
}


def trace_workload_name(canonical_name: str, mode: str) -> str:
    """Return the snapshot identity selected by an explicit experiment mode."""
    if canonical_name not in PYTHON_WORKLOADS:
        raise ValueError(
            f"workload {canonical_name!r} is not in the frozen Python workload set: "
            f"{', '.join(PYTHON_WORKLOADS)}"
        )
    if mode == MODE_INVM_PY:
        return canonical_name
    if mode == MODE_NEXUS_PY:
        return f"{canonical_name}-s3-rpc-shmem"
    if mode == MODE_NEXUS_RDMA_PY:
        return f"{canonical_name}-s3-rpc-rdma"
    if mode in (MODE_INVM_JS, MODE_NEXUS_JS):
        if canonical_name != "helloworld":
            raise ValueError(f"workload {canonical_name!r} has no JavaScript implementation")
        name = "jshelloworld"
        return name if mode == MODE_INVM_JS else f"{name}-s3-rpc-shmem"
    if mode == MODE_HOSTTCP_PY:
        return f"{canonical_name}-s3-rpc-hosttcp"
    if canonical_name not in GO_WORKLOAD_NAMES:
        raise ValueError(f"workload {canonical_name!r} has no Go implementation")
    go_name = GO_WORKLOAD_NAMES[canonical_name]
    if mode == MODE_INVM_GO:
        return go_name
    if mode == MODE_NEXUS_GO:
        return f"{go_name}-s3-rpc-shmem"
    if mode == MODE_NEXUS_RDMA:
        return f"{go_name}-s3-rpc-rdma"
    if mode == MODE_HOSTTCP_GO:
        return f"{go_name}-s3-rpc-hosttcp"
    raise ValueError(f"unsupported experiment mode: {mode}")


def canonical_workload_name(trace_name: str) -> str:
    """Recover the canonical identity before looking up duration/RPS data."""
    stem = trace_name.split("-", 1)[0]
    if stem == "jshelloworld":
        return "helloworld"
    for canonical, go_name in GO_WORKLOAD_NAMES.items():
        if stem == go_name:
            return canonical
    return stem
