package eval

import (
	"strings"
	"testing"
)

func TestUniformRemoteHeadAcceptsConsistentTenants(t *testing.T) {
	const head = "b7bff4266377f674130a9dd0a149812774e715ce"
	got, err := uniformRemoteHead([]remoteRevision{
		{target: "nehalem@10.0.1.7", head: head + "\n"},
		{target: "nehalem@10.0.1.8", head: head},
		{target: "nehalem@10.0.1.9", head: head},
		{target: "nehalem@10.0.1.10", head: head},
	})
	if err != nil || got != head {
		t.Fatalf("uniformRemoteHead() = %q, %v", got, err)
	}
}

func TestUniformRemoteHeadRejectsMissingOrMixedProvenance(t *testing.T) {
	tests := []struct {
		name      string
		revisions []remoteRevision
		want      string
	}{
		{name: "no tenants", want: "no RDMA tenants"},
		{name: "empty head", revisions: []remoteRevision{{target: "tenant-a"}}, want: "tenant-a is empty"},
		{name: "mixed heads", revisions: []remoteRevision{{target: "tenant-a", head: "aaaa"}, {target: "tenant-b", head: "bbbb"}}, want: "tenant-a=aaaa, tenant-b=bbbb"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := uniformRemoteHead(test.revisions); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("uniformRemoteHead() error = %v, want substring %q", err, test.want)
			}
		})
	}
}
