package eval

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// ActivatorIdentity is the campaign-start identity of the Knative activator
// Deployment.  The Deployment UID (rather than a pod UID) remains stable
// across ordinary pod replacement, while generation detects a mutation of
// the Deployment itself.
type ActivatorIdentity struct {
	UID        string `json:"uid"`
	Generation int64  `json:"generation"`
}

// ParseActivatorIdentity parses the exact two-column form emitted by
// CaptureActivatorIdentity.  Keeping this parser strict prevents an empty or
// partially rendered kubectl result from becoming a false baseline.
func ParseActivatorIdentity(output string) (ActivatorIdentity, error) {
	fields := strings.Fields(strings.TrimSpace(output))
	if len(fields) != 2 || !validKubernetesUID(fields[0]) {
		return ActivatorIdentity{}, fmt.Errorf("malformed activator identity %q", strings.TrimSpace(output))
	}
	generation, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil || generation < 1 {
		return ActivatorIdentity{}, fmt.Errorf("malformed activator generation %q", fields[1])
	}
	return ActivatorIdentity{UID: fields[0], Generation: generation}, nil
}

func validKubernetesUID(value string) bool {
	if value == "" || len(value) > 253 || strings.ContainsAny(value, " \t\r\n") {
		return false
	}
	if !isAlphaNumeric(value[0]) || !isAlphaNumeric(value[len(value)-1]) {
		return false
	}
	for _, r := range value {
		if !isAlphaNumeric(byte(r)) && r != '-' && r != '_' && r != '.' {
			return false
		}
	}
	return true
}

func isAlphaNumeric(value byte) bool {
	return (value >= 'a' && value <= 'z') || (value >= 'A' && value <= 'Z') || (value >= '0' && value <= '9')
}

// CaptureActivatorIdentity reads metadata without changing the Deployment or
// its pods.  The command deliberately requests only metadata.uid and
// metadata.generation, so its output has a small, auditable surface.
func CaptureActivatorIdentity(ctx context.Context) (ActivatorIdentity, error) {
	cmd := exec.CommandContext(ctx, "kubectl", "get", "deployment", "activator", "-n", "knative-serving", "-o", "jsonpath={.metadata.uid}{'\\t'}{.metadata.generation}")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return ActivatorIdentity{}, fmt.Errorf("capture activator identity: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return ParseActivatorIdentity(string(output))
}

func (a ActivatorIdentity) Validate() error {
	if !validKubernetesUID(a.UID) {
		return errors.New("activator UID is missing or malformed")
	}
	if a.Generation < 1 {
		return errors.New("activator generation is missing or malformed")
	}
	return nil
}

func (a ActivatorIdentity) Equal(other ActivatorIdentity) bool {
	return a.UID == other.UID && a.Generation == other.Generation
}

// WorkerLeakEvidence records complete command output, not only a count.  A
// clean result uses an explicit empty JSON array; a missing array is rejected
// as incomplete evidence by ValidateFinalLeakCheck.
type WorkerLeakEvidence struct {
	Firecracker   []string `json:"firecracker_processes"`
	KnIntegration []string `json:"kn_integration_processes"`
	NexusBackend  []string `json:"nexus_backend_processes"`
}

type StorageLeakEvidence struct {
	RDMAServer   []string `json:"rdma_server_processes"`
	RDMASessions []string `json:"rdma_sessions"`
}

type KubernetesLeakEvidence struct {
	KSVCCount int `json:"ksvc_count"`
}

type SnapshotLeakEvidence struct {
	Entries []string `json:"entries"`
	Bytes   int64    `json:"bytes"`
}

// FinalLeakCheck is the required immediate post-cleanup evidence artifact.
// It is intentionally a single record so an immutable bundle can bind all
// cleanup observations and the activator identity check together.
type FinalLeakCheck struct {
	Version     int                    `json:"version"`
	Status      string                 `json:"status"`
	CapturedUTC string                 `json:"captured_utc"`
	Errors      []string               `json:"errors,omitempty"`
	Worker      WorkerLeakEvidence     `json:"worker"`
	Storage     StorageLeakEvidence    `json:"storage"`
	Kubernetes  KubernetesLeakEvidence `json:"kubernetes"`
	Snapshots   SnapshotLeakEvidence   `json:"snapshots"`
	Activator   ActivatorIdentity      `json:"activator"`
}

// ValidateFinalLeakCheck rejects both leaks and incomplete evidence.  A
// COMPLETE campaign seal must provide a clean check against the exact
// campaign-start Deployment identity.
func (c FinalLeakCheck) ValidateFinalLeakCheck(baseline ActivatorIdentity) error {
	if err := c.ValidateFinalLeakCheckEvidence(); err != nil {
		return err
	}
	if c.Status != "PASS" {
		return errors.New("final leak check status/version is invalid")
	}
	if err := baseline.Validate(); err != nil {
		return fmt.Errorf("campaign activator baseline: %w", err)
	}
	if !c.Activator.Equal(baseline) {
		return fmt.Errorf("activator identity changed: final=%s/%d baseline=%s/%d", c.Activator.UID, c.Activator.Generation, baseline.UID, baseline.Generation)
	}
	if err := validateWorkerProcessArrays(c.Worker); err != nil {
		return err
	}
	if err := validateStorageProcessArrays(c.Storage); err != nil {
		return err
	}
	if c.Kubernetes.KSVCCount != 0 {
		return fmt.Errorf("Kubernetes ksvc leak count is %d", c.Kubernetes.KSVCCount)
	}
	if c.Snapshots.Entries == nil {
		return errors.New("snapshot entries evidence is missing")
	}
	if c.Snapshots.Bytes < 0 {
		return errors.New("snapshot byte count is negative")
	}
	if len(c.Snapshots.Entries) != 0 || c.Snapshots.Bytes != 0 {
		return fmt.Errorf("snapshot leak remains: entries=%d bytes=%d", len(c.Snapshots.Entries), c.Snapshots.Bytes)
	}
	return nil
}

// ValidateFinalLeakCheckEvidence checks that every required observation was
// captured, while intentionally allowing leaks.  This is used before writing
// a failed artifact so the failure itself remains auditable and immutable.
func (c FinalLeakCheck) ValidateFinalLeakCheckEvidence() error {
	if c.Version != 1 || (c.Status != "PASS" && c.Status != "FAIL") {
		return errors.New("final leak check status/version is invalid")
	}
	if c.CapturedUTC == "" {
		return errors.New("final leak check timestamp is missing")
	}
	if _, err := time.Parse(time.RFC3339Nano, c.CapturedUTC); err != nil {
		return fmt.Errorf("final leak check timestamp is malformed: %w", err)
	}
	if err := c.Activator.Validate(); err != nil {
		return err
	}
	if c.Worker.Firecracker == nil || c.Worker.KnIntegration == nil || c.Worker.NexusBackend == nil {
		return errors.New("worker process evidence is incomplete")
	}
	if c.Storage.RDMAServer == nil || c.Storage.RDMASessions == nil {
		return errors.New("storage RDMA process/session evidence is incomplete")
	}
	if c.Snapshots.Entries == nil {
		return errors.New("snapshot entries evidence is missing")
	}
	if c.Snapshots.Bytes < 0 {
		return errors.New("snapshot byte count is negative")
	}
	return nil
}

func validateWorkerProcessArrays(p WorkerLeakEvidence) error {
	if p.Firecracker == nil || p.KnIntegration == nil || p.NexusBackend == nil {
		return errors.New("worker process evidence is incomplete")
	}
	if len(p.Firecracker) != 0 || len(p.KnIntegration) != 0 || len(p.NexusBackend) != 0 {
		return errors.New("worker process leak remains")
	}
	return nil
}

func validateStorageProcessArrays(p StorageLeakEvidence) error {
	if p.RDMAServer == nil || p.RDMASessions == nil {
		return errors.New("storage RDMA process/session evidence is incomplete")
	}
	if len(p.RDMAServer) != 0 || len(p.RDMASessions) != 0 {
		return errors.New("storage process/session leak remains")
	}
	return nil
}

// WriteFinalLeakCheck is create-only so a failed or missing cleanup check
// cannot be repaired in place after the fact.
func WriteFinalLeakCheck(path string, check FinalLeakCheck, baseline ActivatorIdentity) error {
	if err := check.ValidateFinalLeakCheck(baseline); err != nil {
		return err
	}
	return CreateOnly(path, check)
}

// WriteFinalLeakCheckEvidence persists both clean and failed observations.
// The bundle verifier separately requires a clean PASS result, so callers
// cannot turn a leak into a successful seal by retaining a failure artifact.
func WriteFinalLeakCheckEvidence(path string, check FinalLeakCheck) error {
	if err := check.ValidateFinalLeakCheckEvidence(); err != nil {
		return err
	}
	return CreateOnly(path, check)
}

// ParseActivatorIdentityJSON is useful to fixture-backed callers that retain
// the complete kubectl JSON response while still validating only metadata.
func ParseActivatorIdentityJSON(data []byte) (ActivatorIdentity, error) {
	var value struct {
		Metadata struct {
			UID        string `json:"uid"`
			Generation int64  `json:"generation"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return ActivatorIdentity{}, fmt.Errorf("invalid activator JSON: %w", err)
	}
	identity := ActivatorIdentity{UID: value.Metadata.UID, Generation: value.Metadata.Generation}
	return identity, identity.Validate()
}
