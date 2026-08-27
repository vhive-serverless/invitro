package eval

import (
	"context"
	"slices"
	"testing"
)

func TestValidateResultRoot(t *testing.T) {
	if err := ValidateResultRoot("/mnt/resources/nexus-evaluation/run/e1"); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"relative", "/tmp/run", "/mnt/resources/nexus-evaluation/../other"} {
		if ValidateResultRoot(path) == nil {
			t.Fatalf("unsafe result root accepted: %s", path)
		}
	}
}

func TestRemoteHome(t *testing.T) {
	home, err := RemoteHome("nehalem@worker.example")
	if err != nil || home != "/users/nehalem" {
		t.Fatalf("home = %q, %v", home, err)
	}
	if _, err := RemoteHome("bad target"); err == nil {
		t.Fatal("invalid target accepted")
	}
}

func TestSSHCommandAcceptsAndPinsFirstSeenHostKey(t *testing.T) {
	command, err := SSHCommand(context.Background(), "nehalem@10.0.1.3", "true")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(command.Args, "StrictHostKeyChecking=accept-new") {
		t.Fatalf("SSH options do not enroll an ephemeral cluster host key: %v", command.Args)
	}
}
