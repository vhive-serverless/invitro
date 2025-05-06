package deployment

import (
	"github.com/vhive-serverless/loader/pkg/common"
	"testing"
)

func TestGCRRemove(t *testing.T) {
	t.Skip()

	function := &common.Function{
		Name: "warm-function-5316106119488084471",
	}

	deployer := &gcrDeployer{
		region:    "us-central1",
		functions: []*common.Function{function},
	}
	deployer.Clean()
}
