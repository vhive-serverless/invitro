package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func putFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}

func restoreWritable(path string) {
	_ = filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
		if err == nil {
			if info.IsDir() {
				_ = os.Chmod(p, 0755)
			} else {
				_ = os.Chmod(p, 0644)
			}
		}
		return nil
	})
}

func TestTreeDigestDeterministic(t *testing.T) {
	a := []record{{Path: "z", SHA256: strings.Repeat("a", 64), Bytes: 1}, {Path: "a", SHA256: strings.Repeat("b", 64), Bytes: 2}}
	b := []record{{Path: "a", SHA256: strings.Repeat("b", 64), Bytes: 2}, {Path: "z", SHA256: strings.Repeat("a", 64), Bytes: 1}}
	// Canonicalization sorts the complete "sha256  path" records, independent
	// of the order in which a filesystem walk discovers files.
	if treeDigest(a) != treeDigest(b) {
		t.Fatal("canonical digest depends on record order")
	}
}

func TestScanRejectsSymlinkAndSpecial(t *testing.T) {
	d := t.TempDir()
	putFile(t, filepath.Join(d, "ok"), "ok")
	if err := os.Symlink(filepath.Join(d, "ok"), filepath.Join(d, "link")); err != nil {
		t.Fatal(err)
	}
	if _, err := scan(d, nil); err == nil {
		t.Fatal("scan accepted symlink")
	}
	_ = os.Remove(filepath.Join(d, "link"))
	if err := os.Mkdir(filepath.Join(d, "dir"), 0755); err != nil {
		t.Fatal(err)
	}
	// A directory is valid; this also ensures traversal does not mistake it
	// for a special file.
	if _, err := scan(d, nil); err != nil {
		t.Fatal(err)
	}
}

func TestScanRejectsHardlinkOutsideTree(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "root")
	if err := os.Mkdir(root, 0755); err != nil {
		t.Fatal(err)
	}
	original := filepath.Join(base, "original")
	putFile(t, original, "same inode")
	if err := os.Link(original, filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}
	if _, err := scan(root, nil); err == nil {
		t.Fatal("scan accepted an externally hardlinked file")
	}
}

func TestPlanMaterializeSealVerifyAndTamper(t *testing.T) {
	base := t.TempDir()
	defer restoreWritable(base)
	q := filepath.Join(base, "qualification")
	old := filepath.Join(base, "old")
	newRoot := filepath.Join(base, "new")
	for _, p := range []string{q, old, newRoot} {
		if err := os.MkdirAll(p, 0755); err != nil {
			t.Fatal(err)
		}
	}
	putFile(t, filepath.Join(old, "a.txt"), "alpha")
	putFile(t, filepath.Join(old, "nested", "b.txt"), "beta")
	oldCampaign := `{"status":"ACQUISITION_START","provenance":[{"branch":"main","head":"abc"}],"marker":"<"}`
	oldCampaignPath := filepath.Join(old, "campaign.json")
	putFile(t, oldCampaignPath, oldCampaign)
	if err := planReuse(q, oldCampaignPath, []string{"artifact=" + old}); err != nil {
		t.Fatal(err)
	}
	ledgerPath := filepath.Join(q, ledgerName)
	if _, err := os.Stat(ledgerPath); err != nil {
		t.Fatal(err)
	}
	qInv, err := scan(q, nil)
	if err != nil {
		t.Fatal(err)
	}
	newCampaignPath := filepath.Join(newRoot, "campaign.json")
	manifest := struct {
		Status              string `json:"status"`
		AcquisitionStart    string `json:"acquisition_start"`
		QualificationRoot   string `json:"qualification_root"`
		QualificationSHA256 string `json:"qualification_sha256"`
	}{"ACQUISITION_START", "2026-08-29T00:00:00Z", q, qInv.TreeSHA}
	mb, _ := json.Marshal(manifest)
	putFile(t, newCampaignPath, string(mb))
	if err := materializeReuse(ledgerPath, newCampaignPath, newRoot); err != nil {
		t.Fatal(err)
	}
	srcInv, _ := scan(old, nil)
	dstInv, err := scan(filepath.Join(newRoot, "artifact"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !sameInventory(srcInv, dstInv) {
		t.Fatal("materialized tree differs from source")
	}
	if err := sealBundle(newRoot); err != nil {
		t.Fatal(err)
	}
	if err := verifyBundle(newRoot); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(newRoot, "artifact", "a.txt"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(newRoot, "artifact", "a.txt"), []byte("tampered"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := verifyBundle(newRoot); err == nil {
		t.Fatal("verify accepted tampered file")
	}
}

func TestMaterializeRejectsQualificationBinding(t *testing.T) {
	base := t.TempDir()
	q := filepath.Join(base, "q")
	old := filepath.Join(base, "old")
	n := filepath.Join(base, "n")
	for _, p := range []string{q, old, n} {
		if err := os.MkdirAll(p, 0755); err != nil {
			t.Fatal(err)
		}
	}
	putFile(t, filepath.Join(old, "x"), "x")
	oldCampaign := filepath.Join(old, "campaign.json")
	putFile(t, oldCampaign, `{}`)
	if err := planReuse(q, oldCampaign, []string{"x=" + old}); err != nil {
		t.Fatal(err)
	}
	putFile(t, filepath.Join(n, "campaign.json"), `{"qualification_root":"wrong","qualification_sha256":"bad"}`)
	if err := materializeReuse(filepath.Join(q, ledgerName), filepath.Join(n, "campaign.json"), n); err == nil {
		t.Fatal("materialize accepted wrong qualification binding")
	}
}

func TestVerifyRejectsUnindexedExtra(t *testing.T) {
	d := t.TempDir()
	defer restoreWritable(d)
	putFile(t, filepath.Join(d, "campaign.json"), `{"status":"ACQUISITION_START","acquisition_start":"2026-08-29T00:00:00Z"}`)
	putFile(t, filepath.Join(d, "x"), `x`)
	if err := sealBundle(d); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(d, 0755); err != nil {
		t.Fatal(err)
	}
	putFile(t, filepath.Join(d, "extra"), `extra`)
	if err := verifyBundle(d); err == nil {
		t.Fatal("verify accepted unindexed extra")
	}
}
