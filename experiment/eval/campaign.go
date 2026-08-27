package eval

import (
	"encoding/json"
	"fmt"
	"os"
)

type Campaign struct {
	Status           string       `json:"status"`
	AcquisitionStart string       `json:"acquisition_start"`
	Provenance       []Provenance `json:"provenance"`
}

func RequireCampaign(path string) (Campaign, error) {
	if path == "" {
		return Campaign{}, fmt.Errorf("--campaign-manifest is required for acquisition")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Campaign{}, err
	}
	var campaign Campaign
	if err := json.Unmarshal(data, &campaign); err != nil {
		return Campaign{}, err
	}
	if campaign.AcquisitionStart == "" || campaign.Status != "ACQUISITION_START" {
		return Campaign{}, fmt.Errorf("campaign manifest is not frozen at ACQUISITION_START")
	}
	return campaign, nil
}

func (c Campaign) HeadForBranch(branch string) (string, error) {
	var head string
	for _, provenance := range c.Provenance {
		if provenance.Branch != branch {
			continue
		}
		if provenance.Head == "" {
			return "", fmt.Errorf("campaign provenance for branch %s has no HEAD", branch)
		}
		if head != "" && head != provenance.Head {
			return "", fmt.Errorf("campaign has conflicting HEADs for branch %s", branch)
		}
		head = provenance.Head
	}
	if head == "" {
		return "", fmt.Errorf("campaign has no provenance for branch %s", branch)
	}
	return head, nil
}
