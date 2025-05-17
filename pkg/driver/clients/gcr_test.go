package clients

import (
	"github.com/vhive-serverless/loader/pkg/common"
	"github.com/vhive-serverless/loader/pkg/config"
	"testing"
)

func TestGCRInvoke(t *testing.T) {
	t.Skip()

	function := &common.Function{
		Name:             "warm-function-4318010827780319511",
		Endpoint:         "warm-function-4318010827780319511-328799819690.us-central1.run.app",
		DirigentMetadata: &common.DirigentMetadata{},
	}

	inv := newHTTPInvoker(&config.Configuration{
		LoaderConfiguration: &config.LoaderConfiguration{
			InvokeProtocol: "http1",
			Platform:       "GCR",
		},
		FailureConfiguration: nil,
		DirigentConfiguration: &config.DirigentConfig{
			Backend: "containerd",
		},
		GCRConfiguration: nil,
		TraceDuration:    1,
		Functions: []*common.Function{
			function,
		},
	})

	inv.Invoke(function, &common.RuntimeSpecification{Runtime: 1, Memory: 1})
}
