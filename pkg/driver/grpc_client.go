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

func Invoke(function *common.Function, functions []*common.Function, promptFunctions []*common.Function, runtimeSpec *common.RuntimeSpecification, cfg *config.LoaderConfiguration, invocationID string,
	jobSchedOutputChannel chan *mc.JobSchedRequest, jobSchedInputChannel chan *mc.JobSchedReply) (bool, *mc.ExecutionRecord, *mc.JobExecutionRecord) {
	client_training := cfg.ClientTraining
	if client_training == common.Caerus {
		return invokefunc.CaerusInvoke(function, promptFunctions, runtimeSpec, cfg, invocationID)
	} else if client_training == common.Multi {
		return invokefunc.MultiInvoke(function, promptFunctions, runtimeSpec, cfg, invocationID)
	} else if client_training == common.Knative {
		return invokefunc.KnativeInvoke(function, promptFunctions, runtimeSpec, cfg, invocationID)
	} else if client_training == common.HiveD {
		return invokefunc.HiveDInvoke(functions, promptFunctions, runtimeSpec, cfg, invocationID)
	} else if client_training == common.INFless {
		return invokefunc.INFlessInvoke(functions, promptFunctions, runtimeSpec, cfg, invocationID)
	} else if client_training == common.Elastic {
		return invokefunc.ElasticInvoke(functions, promptFunctions, runtimeSpec, cfg, invocationID)
	} else if client_training == common.ServerfulOptimus {
		return invokefunc.ServerfulOptimusInvoke(function, promptFunctions, runtimeSpec, cfg, invocationID, jobSchedOutputChannel, jobSchedInputChannel)
	} else {
		log.Errorf("Invalid client_training value: %s", client_training)
		return false, nil, nil
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
