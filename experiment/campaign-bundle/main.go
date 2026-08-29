// Command campaign-bundle makes reuse provenance and campaign bundles
// independently inspectable. Lifecycle validation is shared with experiment
// runners through experiment/eval.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/vhive-serverless/loader/experiment/eval"
)

const (
	ledgerName          = "reuse-ledger.json"
	materializationName = "reuse-materialization.json"
	indexName           = "bundle-index.csv"
	sealName            = "bundle-seal.json"
	finalLeakCheckName  = "campaign-final-leak-check.json"
)

type record struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}

type inventory struct {
	Records []record
	Bytes   int64
	TreeSHA string
}

type ledgerEntry struct {
	Destination      string   `json:"destination"`
	SourceRoot       string   `json:"source_root"`
	RegularFileCount int64    `json:"regular_file_count"`
	TotalBytes       int64    `json:"total_bytes"`
	CanonicalTreeSHA string   `json:"canonical_tree_sha256"`
	Inventory        []record `json:"inventory"`
}

type ledger struct {
	Version            int             `json:"version"`
	Status             string          `json:"status"`
	CreatedUTC         string          `json:"created_utc"`
	QualificationRoot  string          `json:"qualification_root"`
	SourceCampaignPath string          `json:"source_campaign_path"`
	SourceCampaignSHA  string          `json:"source_campaign_sha256"`
	SourceCampaignJSON json.RawMessage `json:"source_campaign_json"`
	// RawMessage keeps the campaign inspectable as an object; this companion
	// string preserves the exact source bytes, including whitespace.
	SourceCampaignJSONExact string        `json:"source_campaign_json_exact"`
	Entries                 []ledgerEntry `json:"entries"`
}

type materializedEntry struct {
	Destination           string `json:"destination"`
	SourceRoot            string `json:"source_root"`
	SourceTreeSHA256      string `json:"source_tree_sha256"`
	DestinationTreeSHA256 string `json:"destination_tree_sha256"`
	RegularFileCount      int64  `json:"regular_file_count"`
	TotalBytes            int64  `json:"total_bytes"`
}

type materialization struct {
	Version        int                 `json:"version"`
	Status         string              `json:"status"`
	CreatedUTC     string              `json:"created_utc"`
	LedgerSHA256   string              `json:"ledger_sha256"`
	CampaignSHA256 string              `json:"campaign_sha256"`
	Entries        []materializedEntry `json:"entries"`
}

type bundleSeal struct {
	Version           int    `json:"version"`
	Status            string `json:"status"`
	CreatedUTC        string `json:"created_utc"`
	CampaignSHA256    string `json:"campaign_sha256"`
	FinalLeakCheckSHA string `json:"final_leak_check_sha256"`
	IndexSHA256       string `json:"index_sha256"`
	CanonicalTreeSHA  string `json:"canonical_tree_sha256"`
	RegularFileCount  int64  `json:"regular_file_count"`
	TotalBytes        int64  `json:"total_bytes"`
}

type entryFlags []string

func (e *entryFlags) String() string { return strings.Join(*e, ",") }
func (e *entryFlags) Set(v string) error {
	if v == "" {
		return errors.New("--entry must not be empty")
	}
	*e = append(*e, v)
	return nil
}

func main() {
	if len(os.Args) < 2 {
		fail("subcommand is required")
	}
	var err error
	switch os.Args[1] {
	case "plan-reuse":
		err = commandPlan(os.Args[2:])
	case "materialize-reuse":
		err = commandMaterialize(os.Args[2:])
	case "leak-check":
		err = commandLeakCheck(os.Args[2:])
	case "seal":
		err = commandSeal(os.Args[2:])
	case "verify":
		err = commandVerify(os.Args[2:])
	default:
		fail("unknown subcommand %q", os.Args[1])
	}
	if err != nil {
		fail("%s", err)
	}
}

func commandLeakCheck(args []string) error {
	fs := flag.NewFlagSet("leak-check", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	root := fs.String("campaign-root", "", "frozen campaign root")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *root == "" {
		return errors.New("--campaign-root is required")
	}
	return captureFinalLeakCheck(context.Background(), *root)
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "campaign-bundle: "+format+"\n", args...)
	os.Exit(1)
}

func commandPlan(args []string) error {
	fs := flag.NewFlagSet("plan-reuse", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	q := fs.String("qualification-root", "", "qualification root")
	campaign := fs.String("source-campaign", "", "source campaign manifest")
	var entries entryFlags
	fs.Var(&entries, "entry", "DEST_REL=ABS_SOURCE (repeatable)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *q == "" || *campaign == "" {
		return errors.New("--qualification-root and --source-campaign are required")
	}
	return planReuse(*q, *campaign, entries)
}

func commandMaterialize(args []string) error {
	fs := flag.NewFlagSet("materialize-reuse", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	ledgerPath := fs.String("ledger", "", "reuse ledger")
	manifest := fs.String("campaign-manifest", "", "new campaign manifest")
	root := fs.String("campaign-root", "", "new campaign root")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *ledgerPath == "" || *manifest == "" || *root == "" {
		return errors.New("--ledger, --campaign-manifest, and --campaign-root are required")
	}
	return materializeReuse(*ledgerPath, *manifest, *root)
}

func commandSeal(args []string) error {
	fs := flag.NewFlagSet("seal", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	root := fs.String("campaign-root", "", "campaign root")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *root == "" {
		return errors.New("--campaign-root is required")
	}
	return sealBundle(*root)
}

func commandVerify(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	root := fs.String("campaign-root", "", "campaign root")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *root == "" {
		return errors.New("--campaign-root is required")
	}
	if err := verifyBundle(*root); err != nil {
		return err
	}
	fmt.Println("PASS campaign-bundle verify")
	return nil
}

func absDir(path string) (string, error) {
	p, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	p = filepath.Clean(p)
	st, err := os.Lstat(p)
	if err != nil {
		return "", err
	}
	if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("not a directory: %s", p)
	}
	return p, nil
}

func regularFile(path string) (os.FileInfo, error) {
	st, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if st.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("symlink is not allowed: %s", path)
	}
	if !st.Mode().IsRegular() {
		return nil, fmt.Errorf("special file is not allowed: %s", path)
	}
	if stat, ok := st.Sys().(*syscall.Stat_t); ok && stat.Nlink != 1 {
		return nil, fmt.Errorf("hardlinked file is not allowed: %s", path)
	}
	return st, nil
}

func inodeKey(info os.FileInfo) (string, bool) {
	s, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", false
	}
	return fmt.Sprintf("%d:%d", s.Dev, s.Ino), true
}

// scan computes the canonical inventory.  It rejects all links/specials and
// repeated inodes, including hardlinks, before returning any digest.
func scan(root string, excluded map[string]bool) (inventory, error) {
	root, err := absDir(root)
	if err != nil {
		return inventory{}, err
	}
	seen := make(map[string]string)
	var recs []record
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink is not allowed: %s", rel)
		}
		if d.IsDir() {
			if !info.Mode().IsDir() {
				return fmt.Errorf("special file is not allowed: %s", rel)
			}
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("special file is not allowed: %s", rel)
		}
		if stat, ok := info.Sys().(*syscall.Stat_t); ok && stat.Nlink != 1 {
			return fmt.Errorf("hardlinked file is not allowed: %s", rel)
		}
		if key, ok := inodeKey(info); ok {
			if prior, exists := seen[key]; exists {
				return fmt.Errorf("hardlinked files are not allowed: %s and %s", prior, rel)
			}
			seen[key] = rel
		}
		if excluded != nil && excluded[rel] {
			return nil
		}
		h, n, err := hashFile(path)
		if err != nil {
			return err
		}
		if n != info.Size() {
			return fmt.Errorf("file changed while reading: %s", rel)
		}
		recs = append(recs, record{Path: rel, SHA256: h, Bytes: n})
		return nil
	})
	if err != nil {
		return inventory{}, err
	}
	sort.Slice(recs, func(i, j int) bool { return recs[i].Path < recs[j].Path })
	var total int64
	for _, r := range recs {
		total += r.Bytes
	}
	return inventory{Records: recs, Bytes: total, TreeSHA: treeDigest(recs)}, nil
}

func hashFile(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", n, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

func treeDigest(recs []record) string {
	lines := make([]string, 0, len(recs))
	for _, r := range recs {
		lines = append(lines, fmt.Sprintf("%s  %s\n", r.SHA256, r.Path))
	}
	sort.Strings(lines)
	h := sha256.New()
	for _, line := range lines {
		_, _ = io.WriteString(h, line)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func sameInventory(a, b inventory) bool {
	if a.Bytes != b.Bytes || a.TreeSHA != b.TreeSHA || len(a.Records) != len(b.Records) {
		return false
	}
	for i := range a.Records {
		if a.Records[i] != b.Records[i] {
			return false
		}
	}
	return true
}

func parseEntry(v string) (string, string, error) {
	i := strings.IndexByte(v, '=')
	if i <= 0 || i == len(v)-1 {
		return "", "", fmt.Errorf("invalid --entry %q (want DEST_REL=ABS_SOURCE)", v)
	}
	dest, src := v[:i], v[i+1:]
	if filepath.IsAbs(dest) || dest == "." || strings.ContainsRune(dest, '\x00') {
		return "", "", fmt.Errorf("unsafe destination %q", dest)
	}
	clean := filepath.Clean(dest)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf("unsafe destination %q", dest)
	}
	if filepath.ToSlash(clean) != clean || strings.HasPrefix(dest, "/") {
		return "", "", fmt.Errorf("unsafe destination %q", dest)
	}
	return filepath.ToSlash(clean), src, nil
}

func jsonBytes(v any) ([]byte, error) {
	return json.MarshalIndent(v, "", "  ")
}

func writeCreate(path string, data []byte, mode os.FileMode) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(path)
		}
	}()
	if _, err = f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	ok = true
	return nil
}

func planReuse(qPath, campaignPath string, specs []string) error {
	q, err := absDir(qPath)
	if err != nil {
		return err
	}
	campaignPath, err = filepath.Abs(campaignPath)
	if err != nil {
		return err
	}
	if _, err := regularFile(campaignPath); err != nil {
		return err
	}
	campaignJSON, err := os.ReadFile(campaignPath)
	if err != nil {
		return err
	}
	var jsonCheck any
	if err := json.Unmarshal(campaignJSON, &jsonCheck); err != nil {
		return fmt.Errorf("invalid source campaign JSON: %w", err)
	}
	campaignSHA, _, err := hashFile(campaignPath)
	if err != nil {
		return err
	}
	if len(specs) == 0 {
		return errors.New("at least one --entry is required")
	}
	entries := make([]ledgerEntry, 0, len(specs))
	dests := make(map[string]bool)
	for _, spec := range specs {
		dest, src, err := parseEntry(spec)
		if err != nil {
			return err
		}
		if dests[dest] {
			return fmt.Errorf("duplicate destination: %s", dest)
		}
		if !strings.Contains(dest, "/") && (dest == ledgerName || dest == materializationName || dest == indexName || dest == sealName || dest == finalLeakCheckName) {
			return fmt.Errorf("destination is reserved: %s", dest)
		}
		dests[dest] = true
		src, err = filepath.Abs(src)
		if err != nil {
			return err
		}
		inv1, err := scan(src, nil)
		if err != nil {
			return err
		}
		inv2, err := scan(src, nil)
		if err != nil {
			return err
		}
		if !sameInventory(inv1, inv2) {
			return fmt.Errorf("source changed between consecutive inventories: %s", src)
		}
		entries = append(entries, ledgerEntry{Destination: dest, SourceRoot: src, RegularFileCount: int64(len(inv1.Records)), TotalBytes: inv1.Bytes, CanonicalTreeSHA: inv1.TreeSHA, Inventory: inv1.Records})
	}
	for i := range entries {
		for j := i + 1; j < len(entries); j++ {
			a, b := entries[i].Destination, entries[j].Destination
			if strings.HasPrefix(a, b+"/") || strings.HasPrefix(b, a+"/") {
				return fmt.Errorf("overlapping destinations: %s and %s", a, b)
			}
		}
	}
	// Ensure qualification state is itself traversable before adding the ledger.
	if _, err := scan(q, nil); err != nil {
		return fmt.Errorf("qualification root: %w", err)
	}
	ledger := ledger{Version: 1, Status: "CONTROLLED_TWO_PROVENANCE_REUSE_PLANNED", CreatedUTC: time.Now().UTC().Format(time.RFC3339Nano), QualificationRoot: q, SourceCampaignPath: campaignPath, SourceCampaignSHA: campaignSHA, SourceCampaignJSON: json.RawMessage(campaignJSON), SourceCampaignJSONExact: string(campaignJSON), Entries: entries}
	b, err := jsonBytes(ledger)
	if err != nil {
		return err
	}
	if err := writeCreate(filepath.Join(q, ledgerName), append(b, '\n'), 0600); err != nil {
		return fmt.Errorf("create ledger: %w", err)
	}
	fmt.Printf("planned %s\n", filepath.Join(q, ledgerName))
	return nil
}

func readLedger(path string) (ledger, []byte, string, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return ledger{}, nil, "", err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return ledger{}, nil, "", err
	}
	if _, err := regularFile(path); err != nil {
		return ledger{}, nil, "", err
	}
	var l ledger
	if err := json.Unmarshal(b, &l); err != nil {
		return ledger{}, nil, "", fmt.Errorf("invalid ledger: %w", err)
	}
	if l.Version != 1 || l.Status != "CONTROLLED_TWO_PROVENANCE_REUSE_PLANNED" {
		return ledger{}, nil, "", errors.New("ledger status/version is invalid")
	}
	if l.QualificationRoot == "" || len(l.Entries) == 0 {
		return ledger{}, nil, "", errors.New("ledger is incomplete")
	}
	if l.SourceCampaignJSONExact == "" || sha256Hex([]byte(l.SourceCampaignJSONExact)) != l.SourceCampaignSHA {
		return ledger{}, nil, "", errors.New("ledger source campaign JSON/hash mismatch")
	}
	var exactValue, rawValue any
	if err := json.Unmarshal([]byte(l.SourceCampaignJSONExact), &exactValue); err != nil {
		return ledger{}, nil, "", errors.New("ledger source campaign JSON is invalid")
	}
	if err := json.Unmarshal(l.SourceCampaignJSON, &rawValue); err != nil || !reflect.DeepEqual(exactValue, rawValue) {
		return ledger{}, nil, "", errors.New("ledger source campaign JSON representations disagree")
	}
	return l, b, sha256Hex(b), nil
}

func manifestBinding(path string) (string, string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}
	if _, err := regularFile(path); err != nil {
		return "", "", err
	}
	var m struct {
		Status              string `json:"status"`
		AcquisitionStart    string `json:"acquisition_start"`
		QualificationRoot   string `json:"qualification_root"`
		QualificationSHA256 string `json:"qualification_sha256"`
		ActivatorUID        string `json:"activator_uid"`
		ActivatorGeneration int64  `json:"activator_generation"`
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return "", "", fmt.Errorf("invalid campaign manifest: %w", err)
	}
	if m.Status != "ACQUISITION_START" || m.AcquisitionStart == "" {
		return "", "", errors.New("campaign manifest is not frozen at ACQUISITION_START")
	}
	if err := (eval.ActivatorIdentity{UID: m.ActivatorUID, Generation: m.ActivatorGeneration}).Validate(); err != nil {
		return "", "", fmt.Errorf("campaign activator baseline: %w", err)
	}
	return m.QualificationRoot, m.QualificationSHA256, nil
}

func underRoot(root, path string) bool {
	r, _ := filepath.Abs(root)
	p, _ := filepath.Abs(path)
	rel, err := filepath.Rel(r, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func validateDestinationParents(root, dest string) error {
	parts := strings.Split(filepath.FromSlash(dest), string(filepath.Separator))
	cur := root
	for _, part := range parts[:len(parts)-1] {
		cur = filepath.Join(cur, part)
		st, err := os.Lstat(cur)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if st.Mode()&os.ModeSymlink != 0 || !st.IsDir() {
			return fmt.Errorf("unsafe destination parent: %s", cur)
		}
	}
	return nil
}

func copyTree(src, dst string, expected inventory) error {
	parent := filepath.Dir(dst)
	if err := os.MkdirAll(parent, 0755); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(parent, ".campaign-bundle-copy-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	// Recreate directory entries too, including empty directories.  The
	// inventory hashes regular files, but byte-for-byte tree reuse must not
	// silently discard the source layout.
	if err := filepath.WalkDir(src, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == src {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || (!d.IsDir() && !info.Mode().IsRegular()) {
			return fmt.Errorf("source changed to an unsupported entry: %s", path)
		}
		if d.IsDir() {
			rel, err := filepath.Rel(src, path)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Join(tmp, rel), info.Mode().Perm()); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	for _, r := range expected.Records {
		s := filepath.Join(src, filepath.FromSlash(r.Path))
		d := filepath.Join(tmp, filepath.FromSlash(r.Path))
		if err := os.MkdirAll(filepath.Dir(d), 0755); err != nil {
			return err
		}
		info, err := regularFile(s)
		if err != nil {
			return err
		}
		in, err := os.Open(s)
		if err != nil {
			return err
		}
		out, err := os.OpenFile(d, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
		if err != nil {
			in.Close()
			return err
		}
		_, copyErr := io.Copy(out, in)
		closeOutErr := out.Close()
		closeInErr := in.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeOutErr != nil {
			return closeOutErr
		}
		if closeInErr != nil {
			return closeInErr
		}
	}
	if err := os.Rename(tmp, dst); err != nil {
		return err
	}
	return nil
}

func materializeReuse(ledgerPath, campaignManifest, campaignRoot string) error {
	l, _, ledgerSHA, err := readLedger(ledgerPath)
	if err != nil {
		return err
	}
	ledgerAbs, err := filepath.Abs(ledgerPath)
	if err != nil {
		return err
	}
	qFromLedger, err := filepath.Abs(l.QualificationRoot)
	if err != nil {
		return err
	}
	if filepath.Clean(ledgerAbs) != filepath.Join(qFromLedger, ledgerName) {
		return errors.New("ledger path does not match its qualification root")
	}
	root, err := absDir(campaignRoot)
	if err != nil {
		return err
	}
	manifest, err := filepath.Abs(campaignManifest)
	if err != nil {
		return err
	}
	if filepath.Clean(manifest) != filepath.Join(root, "campaign.json") {
		return errors.New("campaign manifest must be campaign-root/campaign.json")
	}
	qRoot, qSHA, err := manifestBinding(manifest)
	if err != nil {
		return err
	}
	qAbs := qFromLedger
	if qRoot != qAbs {
		return fmt.Errorf("campaign qualification_root does not match ledger")
	}
	qInv, err := scan(qAbs, nil)
	if err != nil {
		return fmt.Errorf("qualification root: %w", err)
	}
	if qSHA != qInv.TreeSHA {
		return errors.New("campaign qualification_sha256 does not match current qualification root")
	}
	campaignSHA, _, err := hashFile(manifest)
	if err != nil {
		return err
	}
	oldCampaign, err := os.ReadFile(l.SourceCampaignPath)
	if err != nil {
		return fmt.Errorf("source campaign: %w", err)
	}
	if _, err := regularFile(l.SourceCampaignPath); err != nil {
		return err
	}
	if sha256Hex(oldCampaign) != l.SourceCampaignSHA || string(oldCampaign) != l.SourceCampaignJSONExact {
		return errors.New("source campaign changed since reuse was planned")
	}
	if _, err := regularFile(filepath.Join(root, materializationName)); err == nil {
		return errors.New("reuse materialization already exists")
	} else if !os.IsNotExist(err) {
		return err
	}
	result := materialization{Version: 1, Status: "CONTROLLED_TWO_PROVENANCE_REUSE_MATERIALIZED", CreatedUTC: time.Now().UTC().Format(time.RFC3339Nano), LedgerSHA256: ledgerSHA, CampaignSHA256: campaignSHA}
	for _, e := range l.Entries {
		if _, _, err := parseEntry(e.Destination + "=" + e.SourceRoot); err != nil {
			return err
		}
		inv1, err := scan(e.SourceRoot, nil)
		if err != nil {
			return err
		}
		if !inventoryMatchesEntry(inv1, e) {
			return fmt.Errorf("source inventory no longer matches ledger: %s", e.SourceRoot)
		}
		dst := filepath.Join(root, filepath.FromSlash(e.Destination))
		if !underRoot(root, dst) {
			return fmt.Errorf("unsafe destination: %s", e.Destination)
		}
		if err := validateDestinationParents(root, e.Destination); err != nil {
			return err
		}
		if _, err := os.Lstat(dst); err == nil {
			return fmt.Errorf("destination already exists: %s", e.Destination)
		} else if !os.IsNotExist(err) {
			return err
		}
		if err := copyTree(e.SourceRoot, dst, inv1); err != nil {
			return err
		}
		dInv, err := scan(dst, nil)
		if err != nil {
			return err
		}
		if !sameInventory(inv1, dInv) {
			return fmt.Errorf("destination inventory differs: %s", e.Destination)
		}
		inv2, err := scan(e.SourceRoot, nil)
		if err != nil {
			return err
		}
		if !inventoryMatchesEntry(inv2, e) {
			return fmt.Errorf("source changed during copy: %s", e.SourceRoot)
		}
		result.Entries = append(result.Entries, materializedEntry{Destination: e.Destination, SourceRoot: e.SourceRoot, SourceTreeSHA256: inv1.TreeSHA, DestinationTreeSHA256: dInv.TreeSHA, RegularFileCount: int64(len(dInv.Records)), TotalBytes: dInv.Bytes})
	}
	b, err := jsonBytes(result)
	if err != nil {
		return err
	}
	if err := writeCreate(filepath.Join(root, materializationName), append(b, '\n'), 0600); err != nil {
		return fmt.Errorf("create materialization: %w", err)
	}
	fmt.Printf("materialized %s\n", filepath.Join(root, materializationName))
	return nil
}

func inventoryMatchesEntry(inv inventory, e ledgerEntry) bool {
	want := inventory{Records: e.Inventory, Bytes: e.TotalBytes, TreeSHA: e.CanonicalTreeSHA}
	return e.RegularFileCount == int64(len(e.Inventory)) && sameInventory(inv, want)
}

func sha256Hex(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

func campaignInRoot(root string) (string, string, error) {
	p := filepath.Join(root, "campaign.json")
	if _, err := regularFile(p); err != nil {
		return "", "", fmt.Errorf("campaign.json: %w", err)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return "", "", err
	}
	var campaign struct {
		Status              string `json:"status"`
		AcquisitionStart    string `json:"acquisition_start"`
		ActivatorUID        string `json:"activator_uid"`
		ActivatorGeneration int64  `json:"activator_generation"`
	}
	if err := json.Unmarshal(b, &campaign); err != nil || campaign.Status != "ACQUISITION_START" || campaign.AcquisitionStart == "" {
		return "", "", errors.New("campaign.json is not frozen at ACQUISITION_START")
	}
	if err := (eval.ActivatorIdentity{UID: campaign.ActivatorUID, Generation: campaign.ActivatorGeneration}).Validate(); err != nil {
		return "", "", fmt.Errorf("campaign activator baseline: %w", err)
	}
	h, _, err := hashFile(p)
	return p, h, err
}

func campaignBaseline(root string) (eval.ActivatorIdentity, error) {
	p := filepath.Join(root, "campaign.json")
	b, err := os.ReadFile(p)
	if err != nil {
		return eval.ActivatorIdentity{}, err
	}
	var campaign struct {
		ActivatorUID        string `json:"activator_uid"`
		ActivatorGeneration int64  `json:"activator_generation"`
	}
	if err := json.Unmarshal(b, &campaign); err != nil {
		return eval.ActivatorIdentity{}, fmt.Errorf("invalid campaign manifest: %w", err)
	}
	identity := eval.ActivatorIdentity{UID: campaign.ActivatorUID, Generation: campaign.ActivatorGeneration}
	if err := identity.Validate(); err != nil {
		return eval.ActivatorIdentity{}, fmt.Errorf("campaign activator baseline: %w", err)
	}
	return identity, nil
}

type frozenCampaign struct {
	Status              string     `json:"status"`
	AcquisitionStart    string     `json:"acquisition_start"`
	ActivatorUID        string     `json:"activator_uid"`
	ActivatorGeneration int64      `json:"activator_generation"`
	Topology            eval.Setup `json:"topology"`
}

func readFrozenCampaign(root string) (frozenCampaign, error) {
	path := filepath.Join(root, "campaign.json")
	if _, err := regularFile(path); err != nil {
		return frozenCampaign{}, fmt.Errorf("campaign.json: %w", err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return frozenCampaign{}, err
	}
	var campaign frozenCampaign
	if err := json.Unmarshal(b, &campaign); err != nil {
		return frozenCampaign{}, fmt.Errorf("invalid campaign manifest: %w", err)
	}
	if campaign.Status != "ACQUISITION_START" || campaign.AcquisitionStart == "" {
		return frozenCampaign{}, errors.New("campaign manifest is not frozen at ACQUISITION_START")
	}
	if err := (eval.ActivatorIdentity{UID: campaign.ActivatorUID, Generation: campaign.ActivatorGeneration}).Validate(); err != nil {
		return frozenCampaign{}, fmt.Errorf("campaign activator baseline: %w", err)
	}
	return campaign, nil
}

func frozenTargets(setup eval.Setup, label string) ([]string, error) {
	ips := setup.LabeledIPs(label)
	if len(ips) == 0 {
		return nil, fmt.Errorf("campaign topology has no %s nodes", label)
	}
	targets := make([]string, 0, len(ips))
	for _, ip := range ips {
		target, err := setup.URLForIP(ip)
		if err != nil {
			return nil, err
		}
		if _, err := eval.RemoteHome(target); err != nil {
			return nil, err
		}
		targets = append(targets, target)
	}
	return targets, nil
}

func workerProcessProbeCommand() []string  { return []string{"ps", "-eo", "pid=,args="} }
func storageProcessProbeCommand() []string { return []string{"ps", "-eo", "pid=,args="} }
func storageSessionProbeCommand() []string { return []string{"ss", "-Htanp"} }
func ksvcProbeCommand() []string           { return []string{"kubectl", "get", "ksvc", "-A", "-o", "name"} }
func snapshotListProbeCommand(path string) []string {
	return []string{"find", path, "-mindepth", "1", "!", "-name", ".gitkeep", "-print"}
}
func snapshotSizeProbeCommand(path string) []string { return []string{"stat", "-c", "%s", "--", path} }

var (
	readonlySSHProbe       = readonlySSH
	activatorIdentityProbe = eval.CaptureActivatorIdentity
	ksvcProbe              = probeKSVC
)

func probeKSVC(ctx context.Context) (string, error) {
	args := ksvcProbeCommand()
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func readonlySSH(ctx context.Context, target string, args ...string) (string, error) {
	cmd, err := eval.SSHCommand(ctx, target, args...)
	if err != nil {
		return "", err
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return strings.TrimSpace(string(output)), fmt.Errorf("read-only SSH probe %s: %w: %s", target, err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
}

func processMatches(output string, names ...string) []string {
	var matches []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		lower := strings.ToLower(line)
		for _, name := range names {
			if strings.Contains(lower, strings.ToLower(name)) {
				matches = append(matches, line)
				break
			}
		}
	}
	return matches
}

func nonEmptyLines(output string) []string {
	var lines []string
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func parseSnapshotSize(output string) (int64, error) {
	value := strings.TrimSpace(output)
	bytes, err := strconv.ParseInt(value, 10, 64)
	if err != nil || bytes < 0 {
		return 0, fmt.Errorf("malformed snapshot byte count %q", value)
	}
	return bytes, nil
}

func captureFinalLeakCheck(ctx context.Context, root string) error {
	campaign, err := readFrozenCampaign(root)
	if err != nil {
		return err
	}
	baseline := eval.ActivatorIdentity{UID: campaign.ActivatorUID, Generation: campaign.ActivatorGeneration}
	workers, err := frozenTargets(campaign.Topology, "loader-nodetype=worker")
	if err != nil {
		return fmt.Errorf("worker targets: %w", err)
	}
	storage, err := frozenTargets(campaign.Topology, "minio-type=tenant")
	if err != nil {
		return fmt.Errorf("storage targets: %w", err)
	}
	check := eval.FinalLeakCheck{Version: 1, Status: "PASS", CapturedUTC: time.Now().UTC().Format(time.RFC3339Nano),
		Worker:     eval.WorkerLeakEvidence{Firecracker: []string{}, KnIntegration: []string{}, NexusBackend: []string{}},
		Storage:    eval.StorageLeakEvidence{RDMAServer: []string{}, RDMASessions: []string{}},
		Kubernetes: eval.KubernetesLeakEvidence{KSVCCount: 0}, Snapshots: eval.SnapshotLeakEvidence{Entries: []string{}, Bytes: 0}, Activator: baseline}
	var probeErrs []error
	for _, target := range workers {
		output, probeErr := readonlySSHProbe(ctx, target, workerProcessProbeCommand()...)
		if probeErr != nil {
			probeErrs = append(probeErrs, probeErr)
			check.Errors = append(check.Errors, probeErr.Error())
			continue
		}
		check.Worker.Firecracker = append(check.Worker.Firecracker, processMatches(output, "firecracker")...)
		check.Worker.KnIntegration = append(check.Worker.KnIntegration, processMatches(output, "kn-integration")...)
		check.Worker.NexusBackend = append(check.Worker.NexusBackend, processMatches(output, "nexus-backend")...)
		home, homeErr := eval.RemoteHome(target)
		if homeErr != nil {
			probeErrs = append(probeErrs, homeErr)
			check.Errors = append(check.Errors, homeErr.Error())
			continue
		}
		probeOutput, snapshotErr := readonlySSHProbe(ctx, target, snapshotListProbeCommand(filepath.Join(home, "khala/runtime/snapshots"))...)
		if snapshotErr != nil {
			probeErrs = append(probeErrs, snapshotErr)
			check.Errors = append(check.Errors, snapshotErr.Error())
			continue
		}
		for _, entry := range nonEmptyLines(probeOutput) {
			check.Snapshots.Entries = append(check.Snapshots.Entries, target+":"+entry)
			sizeOutput, sizeErr := readonlySSHProbe(ctx, target, snapshotSizeProbeCommand(entry)...)
			if sizeErr != nil {
				probeErrs = append(probeErrs, sizeErr)
				check.Errors = append(check.Errors, sizeErr.Error())
				continue
			}
			bytes, parseErr := parseSnapshotSize(sizeOutput)
			if parseErr != nil {
				probeErrs = append(probeErrs, parseErr)
				check.Errors = append(check.Errors, parseErr.Error())
				continue
			}
			check.Snapshots.Bytes += bytes
		}
	}
	for _, target := range storage {
		output, probeErr := readonlySSHProbe(ctx, target, storageProcessProbeCommand()...)
		if probeErr != nil {
			probeErrs = append(probeErrs, probeErr)
			check.Errors = append(check.Errors, probeErr.Error())
			continue
		}
		check.Storage.RDMAServer = append(check.Storage.RDMAServer, processMatches(output, "s3-rdma-server")...)
		sessions, sessionErr := readonlySSHProbe(ctx, target, storageSessionProbeCommand()...)
		if sessionErr != nil {
			probeErrs = append(probeErrs, sessionErr)
			check.Errors = append(check.Errors, sessionErr.Error())
		} else {
			check.Storage.RDMASessions = append(check.Storage.RDMASessions, processMatches(sessions, "s3-rdma-server", "rdma", ":10090", ":10191")...)
		}
	}
	ksvcOutput, ksvcErr := ksvcProbe(ctx)
	if ksvcErr != nil {
		probeErrs = append(probeErrs, ksvcErr)
		check.Errors = append(check.Errors, fmt.Sprintf("kubectl ksvc probe: %v", ksvcErr))
	} else {
		for _, line := range strings.Split(strings.TrimSpace(string(ksvcOutput)), "\n") {
			if strings.TrimSpace(line) != "" {
				check.Kubernetes.KSVCCount++
			}
		}
	}
	finalActivator, activatorErr := activatorIdentityProbe(ctx)
	if activatorErr != nil {
		probeErrs = append(probeErrs, activatorErr)
		check.Errors = append(check.Errors, activatorErr.Error())
	} else {
		check.Activator = finalActivator
	}
	cleanErr := check.ValidateFinalLeakCheck(baseline)
	if len(probeErrs) > 0 && cleanErr == nil {
		cleanErr = errors.New("one or more final leak-check probes failed")
	}
	if cleanErr != nil {
		check.Status = "FAIL"
	}
	path := filepath.Join(root, finalLeakCheckName)
	writeErr := eval.WriteFinalLeakCheckEvidence(path, check)
	if writeErr != nil {
		probeErrs = append(probeErrs, cleanErr, writeErr)
		return errors.Join(probeErrs...)
	}
	if cleanErr != nil {
		probeErrs = append(probeErrs, cleanErr)
		return errors.Join(probeErrs...)
	}
	fmt.Printf("captured %s\n", path)
	return nil
}

func readFinalLeakCheck(root string, baseline eval.ActivatorIdentity) error {
	path := filepath.Join(root, finalLeakCheckName)
	if _, err := regularFile(path); err != nil {
		return fmt.Errorf("final leak-check evidence: %w", err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("final leak-check evidence: %w", err)
	}
	var check eval.FinalLeakCheck
	if err := json.Unmarshal(b, &check); err != nil {
		return fmt.Errorf("invalid final leak-check evidence: %w", err)
	}
	if err := check.ValidateFinalLeakCheck(baseline); err != nil {
		return fmt.Errorf("final leak-check evidence: %w", err)
	}
	return nil
}

func finalLeakCheckHash(root string, baseline eval.ActivatorIdentity) (string, error) {
	if err := readFinalLeakCheck(root, baseline); err != nil {
		return "", err
	}
	path := filepath.Join(root, finalLeakCheckName)
	hash, _, err := hashFile(path)
	return hash, err
}

func sealBundle(campaignRoot string) error {
	root, err := absDir(campaignRoot)
	if err != nil {
		return err
	}
	for _, name := range []string{indexName, sealName} {
		if _, err := os.Lstat(filepath.Join(root, name)); err == nil {
			return fmt.Errorf("%s already exists", name)
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	_, campaignSHA, err := campaignInRoot(root)
	if err != nil {
		return err
	}
	baseline, err := campaignBaseline(root)
	if err != nil {
		return err
	}
	leakCheckSHA, err := finalLeakCheckHash(root, baseline)
	if err != nil {
		return err
	}
	inv, err := scan(root, map[string]bool{indexName: true, sealName: true})
	if err != nil {
		return err
	}
	indexBytes, err := makeIndex(inv.Records)
	if err != nil {
		return err
	}
	if err := writeCreate(filepath.Join(root, indexName), indexBytes, 0444); err != nil {
		return err
	}
	seal := bundleSeal{Version: 1, Status: "IMMUTABLE_COMPLETE", CreatedUTC: time.Now().UTC().Format(time.RFC3339Nano), CampaignSHA256: campaignSHA, FinalLeakCheckSHA: leakCheckSHA, IndexSHA256: sha256Hex(indexBytes), CanonicalTreeSHA: inv.TreeSHA, RegularFileCount: int64(len(inv.Records)), TotalBytes: inv.Bytes}
	b, err := jsonBytes(seal)
	if err != nil {
		return err
	}
	if err := writeCreate(filepath.Join(root, sealName), append(b, '\n'), 0444); err != nil {
		return err
	}
	if err := removeWriteBits(root); err != nil {
		return err
	}
	fmt.Printf("sealed %s\n", filepath.Join(root, sealName))
	return nil
}

func makeIndex(recs []record) ([]byte, error) {
	var b strings.Builder
	w := csv.NewWriter(&b)
	if err := w.Write([]string{"path", "sha256", "bytes"}); err != nil {
		return nil, err
	}
	for _, r := range recs {
		if err := w.Write([]string{r.Path, r.SHA256, fmt.Sprintf("%d", r.Bytes)}); err != nil {
			return nil, err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	return []byte(b.String()), nil
}

func removeWriteBits(root string) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return os.Chmod(path, info.Mode().Perm()&^0222)
	})
}

func verifyBundle(campaignRoot string) error {
	root, err := absDir(campaignRoot)
	if err != nil {
		return err
	}
	idxPath := filepath.Join(root, indexName)
	sealPath := filepath.Join(root, sealName)
	idxBytes, err := os.ReadFile(idxPath)
	if err != nil {
		return err
	}
	sealBytes, err := os.ReadFile(sealPath)
	if err != nil {
		return err
	}
	var seal bundleSeal
	if err := json.Unmarshal(sealBytes, &seal); err != nil {
		return fmt.Errorf("invalid seal: %w", err)
	}
	if seal.Version != 1 || seal.Status != "IMMUTABLE_COMPLETE" {
		return errors.New("seal status/version is invalid")
	}
	_, campaignSHA, err := campaignInRoot(root)
	if err != nil {
		return err
	}
	baseline, err := campaignBaseline(root)
	if err != nil {
		return err
	}
	leakCheckSHA, err := finalLeakCheckHash(root, baseline)
	if err != nil {
		return err
	}
	if campaignSHA != seal.CampaignSHA256 {
		return errors.New("campaign hash mismatch")
	}
	if sha256Hex(idxBytes) != seal.IndexSHA256 {
		return errors.New("index hash mismatch")
	}
	if seal.FinalLeakCheckSHA == "" || leakCheckSHA != seal.FinalLeakCheckSHA {
		return errors.New("final leak-check hash mismatch")
	}
	want, err := parseIndex(idxBytes)
	if err != nil {
		return err
	}
	inv, err := scan(root, map[string]bool{indexName: true, sealName: true})
	if err != nil {
		return err
	}
	if !sameInventory(inv, inventory{Records: want, Bytes: sumBytes(want), TreeSHA: treeDigest(want)}) {
		return errors.New("bundle files do not match index")
	}
	if inv.TreeSHA != seal.CanonicalTreeSHA || int64(len(inv.Records)) != seal.RegularFileCount || inv.Bytes != seal.TotalBytes {
		return errors.New("seal inventory mismatch")
	}
	var writeErr error
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, e := d.Info()
		if e != nil {
			return e
		}
		if info.Mode().Perm()&0222 != 0 {
			writeErr = fmt.Errorf("write bit remains: %s", path)
			return nil
		}
		return nil
	})
	if err != nil {
		return err
	}
	if writeErr != nil {
		return writeErr
	}
	return nil
}

func parseIndex(data []byte) ([]record, error) {
	r := csv.NewReader(strings.NewReader(string(data)))
	header, err := r.Read()
	if err != nil {
		return nil, err
	}
	if len(header) != 3 || header[0] != "path" || header[1] != "sha256" || header[2] != "bytes" {
		return nil, errors.New("invalid bundle index header")
	}
	var out []record
	seen := make(map[string]bool)
	for {
		row, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if len(row) != 3 || row[0] == "" || filepath.IsAbs(filepath.FromSlash(row[0])) || row[0] == indexName || row[0] == sealName {
			return nil, errors.New("invalid bundle index path")
		}
		clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(row[0])))
		if clean != row[0] || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || seen[clean] {
			return nil, errors.New("unsafe or duplicate bundle index path")
		}
		decoded, decodeErr := hex.DecodeString(row[1])
		if decodeErr != nil || len(decoded) != sha256.Size {
			return nil, errors.New("invalid bundle index hash")
		}
		n, err := strconv.ParseInt(row[2], 10, 64)
		if err != nil || n < 0 {
			return nil, errors.New("invalid bundle index byte count")
		}
		seen[clean] = true
		out = append(out, record{Path: clean, SHA256: row[1], Bytes: n})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

func sumBytes(recs []record) int64 {
	var n int64
	for _, r := range recs {
		n += r.Bytes
	}
	return n
}
