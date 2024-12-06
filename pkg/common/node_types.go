package common

type NodeGroup struct {
    MasterNode     string
    AutoScalerNode string
    ActivatorNode  string
    LoaderNode     string
    WorkerNodes    []string
}