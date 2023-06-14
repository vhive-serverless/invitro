package driver

import (
	"strings"

	"github.com/eth-easl/loader/pkg/common"
	"github.com/eth-easl/loader/pkg/config"

	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"

	invokefunc "github.com/eth-easl/loader/pkg/driver/invokefunc"
	mc "github.com/eth-easl/loader/pkg/metric"
)

func Invoke(function *common.Function, runtimeSpec *common.RuntimeSpecification, cfg *config.LoaderConfiguration) (bool, *mc.ExecutionRecord) {
	// log.Tracef("(Invoke)\t %s: %d[ms], %d[MiB]", function.Name, runtimeSpec.Runtime, runtimeSpec.Memory)
	// runtimeSpec.Stats.Iterations = 5
	client_training := cfg.ClientTraining
	if client_training == common.Batch {
		return invokefunc.BatchInvoke(function, runtimeSpec, cfg)
	} else if client_training == common.BatchPriority {
		return invokefunc.BatchPriorityInvoke(function, runtimeSpec, cfg)
	} else if client_training == common.PipelineBatchPriority {
		return invokefunc.PipelineBatchPriorityInvoke(function, runtimeSpec, cfg)
	} else if client_training == common.Single {
		return invokefunc.SingleInvoke(function, runtimeSpec, cfg)
	} else {
		log.Errorf("Invalid client_training value: %s", client_training)
		return false, nil
	}
}

func extractInstanceName(data string) string {
	indexOfHyphen := strings.LastIndex(data, common.FunctionNamePrefix)
	if indexOfHyphen == -1 {
		return data
	}

	return data[indexOfHyphen:]
}

func gRPCConnectionClose(conn *grpc.ClientConn) {
	if conn == nil {
		return
	}

	if err := conn.Close(); err != nil {
		log.Warnf("Error while closing gRPC connection - %s\n", err)
	}
}
