package eval

import (
	"fmt"
	"os/exec"
	"strings"
)

type Provenance struct {
	Repository string            `json:"repository,omitempty"`
	Head       string            `json:"head,omitempty"`
	Branch     string            `json:"branch,omitempty"`
	Status     string            `json:"status,omitempty"`
	Artifacts  map[string]string `json:"artifacts,omitempty"`
}

func GitProvenance(path string) (Provenance, error) {
	p := Provenance{Repository: path, Artifacts: map[string]string{}}
	for key, args := range map[string][]string{"Head": {"rev-parse", "HEAD"}, "Branch": {"branch", "--show-current"}, "Status": {"status", "--porcelain"}} {
		out, err := exec.Command("git", append([]string{"-C", path}, args...)...).Output()
		if err != nil {
			return p, err
		}
		value := strings.TrimSpace(string(out))
		switch key {
		case "Head":
			p.Head = value
		case "Branch":
			p.Branch = value
		case "Status":
			p.Status = value
		}
	}
	return p, nil
}
func (p Provenance) ValidateClean() error {
	if p.Head == "" {
		return fmt.Errorf("missing repository HEAD")
	}
	if p.Status != "" {
		return fmt.Errorf("repository %s is dirty", p.Repository)
	}
	return nil
}
