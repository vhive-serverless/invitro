package eval

import "testing"

func TestHeadForBranchAcceptsRepeatedMatchingDeployment(t *testing.T) {
	campaign := Campaign{Provenance: []Provenance{
		{Branch: "jy/asplos-26", Head: "abc"},
		{Branch: "jy/asplos-26", Head: "abc"},
	}}
	head, err := campaign.HeadForBranch("jy/asplos-26")
	if err != nil || head != "abc" {
		t.Fatalf("head = %q, %v", head, err)
	}
}

func TestHeadForBranchRejectsConflictingDeployment(t *testing.T) {
	campaign := Campaign{Provenance: []Provenance{
		{Branch: "jy/asplos-26", Head: "abc"},
		{Branch: "jy/asplos-26", Head: "def"},
	}}
	if _, err := campaign.HeadForBranch("jy/asplos-26"); err == nil {
		t.Fatal("conflicting campaign provenance accepted")
	}
}
