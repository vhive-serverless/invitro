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

func Invoke(function *common.Function, functions []*common.Function, promptFunctions []*common.Function, runtimeSpec *common.RuntimeSpecification, cfg *config.LoaderConfiguration, invocationID string) (bool, *mc.ExecutionRecord) {
	client_training := cfg.ClientTraining
	// runtimeSpec.Runtime = runtimeSpec.Runtime * 5
	if client_training == common.Batch {
		return invokefunc.BatchInvoke(function, promptFunctions, runtimeSpec, cfg, invocationID)
	} else if client_training == common.BatchPriority {
		return invokefunc.BatchPriorityInvoke(function, promptFunctions, runtimeSpec, cfg, invocationID)
	} else if client_training == common.PipelineBatchPriority {
		return invokefunc.PipelineBatchPriorityInvoke(function, promptFunctions, runtimeSpec, cfg, invocationID)
	} else if client_training == common.Single {
		return invokefunc.SingleInvoke(function, promptFunctions, runtimeSpec, cfg, invocationID)
	} else if client_training == common.HiveD {
		return invokefunc.HiveDInvoke(functions, promptFunctions, runtimeSpec, cfg, invocationID)
	} else if client_training == common.HiveDElastic {
		return invokefunc.HiveDElasticInvoke(functions, promptFunctions, runtimeSpec, cfg, invocationID)
	} else if client_training == common.Elastic {
		return invokefunc.ElasticInvoke(functions, promptFunctions, runtimeSpec, cfg, invocationID)
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
