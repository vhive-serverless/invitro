package eval

import (
	"testing"
)

func TestExpectedProfiles(t *testing.T) {
	for profile, want := range map[Profile]Counts{
		Profile4: {1, 1, 1, 1}, Profile10: {1, 1, 4, 4},
		Profile14: {1, 1, 6, 6}, Profile18: {1, 1, 8, 8},
	} {
		got, err := ExpectedCounts(profile)
		if err != nil || got != want {
			t.Fatalf("ExpectedCounts(%q) = %+v, %v; want %+v", profile, got, err, want)
		}
	}
}

func TestFourNodePairingUsesLabeledWorkerOnly(t *testing.T) {
	setup := Setup{}
	setup.NodeLabel = map[string][]string{"loader-nodetype=worker": {"10.0.1.3"}, "minio-type=tenant": {"10.0.1.4"}}
	got := Pairing(setup)
	if len(got) != 1 || got["10.0.1.3"] != "10.0.1.4" {
		t.Fatalf("Pairing = %#v", got)
	}
}

func TestNormalizeCanonicalMinio(t *testing.T) {
	base, endpoint, err := NormalizeMinioEndpoint("http://" + CanonicalMinioHost)
	if err != nil || base != "http://"+CanonicalMinioHost+":80" || endpoint != CanonicalMinioHost+":80" {
		t.Fatalf("Normalize = %q %q %v", base, endpoint, err)
	}
	if _, _, err := NormalizeMinioEndpoint("https://" + CanonicalMinioHost); err == nil {
		t.Fatal("accepted TLS endpoint")
	}
}

func TestParseLiveNodes(t *testing.T) {
	data := []byte(`{"items":[{"metadata":{"name":"worker","labels":{"loader-nodetype":"worker","minio-type":""}},"status":{"addresses":[{"type":"InternalIP","address":"10.0.1.3"}],"conditions":[{"type":"Ready","status":"True"}]}}]}`)
	nodes, err := ParseLiveNodes(data)
	if err != nil || len(nodes) != 1 || nodes[0].InternalIP != "10.0.1.3" || nodes[0].Ready != "True" {
		t.Fatalf("ParseLiveNodes = %+v, %v", nodes, err)
	}
}
