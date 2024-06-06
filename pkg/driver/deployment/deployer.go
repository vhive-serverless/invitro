package deployment

import "github.com/vhive-serverless/loader/pkg/common"

type FunctionDeployer interface {
	Deploy(functions []*common.Function, configuration interface{})
	Clean()
}
