package deployment

import (
	"os"
	"strings"
	"testing"

	"github.com/vhive-serverless/loader/pkg/common"
)

func TestKnbillHashes(t *testing.T) {
	function := &common.Function{
		InvocationStats: &common.FunctionInvocationStats{
			HashOwner:    "owner-hash",
			HashApp:      "app-hash",
			HashFunction: "function-hash",
		},
	}

	hashOwner, hashApp, hashFunction := knbillHashes(function)
	if hashOwner != "owner-hash" || hashApp != "app-hash" || hashFunction != "function-hash" {
		t.Fatalf("knbillHashes = %q, %q, %q; want trace hashes", hashOwner, hashApp, hashFunction)
	}
}

func TestKnbillHashesWithoutInvocationStats(t *testing.T) {
	hashOwner, hashApp, hashFunction := knbillHashes(&common.Function{})
	if hashOwner != "" || hashApp != "" || hashFunction != "" {
		t.Fatalf("knbillHashes = %q, %q, %q; want empty hashes", hashOwner, hashApp, hashFunction)
	}
}

func TestTraceTemplatesIncludeBillingAnnotations(t *testing.T) {
	templates := []string{
		"../../../workloads/container/trace_func_go.yaml",
		"../../../workloads/firecracker/trace_func_go.yaml",
		"../../trace/test_data/service.yaml",
	}
	required := []string{
		"knbill.dev/user-id: $KNBILL_HASH_OWNER",
		"knbill.dev/app-id: $KNBILL_HASH_APP",
		"knbill.dev/func-id: $KNBILL_HASH_FUNCTION",
	}

	for _, path := range templates {
		t.Run(path, func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			content := string(data)
			for _, expected := range required {
				if !strings.Contains(content, expected) {
					t.Fatalf("%s missing %q", path, expected)
				}
			}
		})
	}
}
