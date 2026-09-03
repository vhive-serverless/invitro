package eval

import (
	"context"
	"fmt"
	"strings"
)

const (
	KhalaBranch       = "jy/asplos-26-nostream"
	InVitroBranch     = "jy/khala-asplos-27-nostream"
	RDMABranch        = "jy/nexus-rdma-eval"
	RDMAOrigin        = "https://github.com/hyscale-lab/rdma-demo.git"
	FirecrackerBranch = "firecracker-v1.14-nexus-shmem-vsock"
	FirecrackerHead   = "ce43c37f475100d3aba1ff5995f88ca6f9a0e5ad"
	FirecrackerOrigin = "https://github.com/JooyoungPark73/firecracker.git"
	FirecrackerSHA256 = "8b61f895b4c14bf253bb27451669caddbee6fd5c1b61dc30a029e285cba31db2"
	KernelSHA256      = "41ce4c9dd77f7d1f8ffd42a545135d6547eafb3ac0d6355fe3eff188af2f949c"
)

type EvaluationHeads struct {
	Khala, InVitro, RDMA, Firecracker string
}

func (c Campaign) EvaluationHeads() (EvaluationHeads, error) {
	khala, err := c.HeadForBranch(KhalaBranch)
	if err != nil {
		return EvaluationHeads{}, err
	}
	invitro, err := c.HeadForBranch(InVitroBranch)
	if err != nil {
		return EvaluationHeads{}, err
	}
	rdma, err := c.HeadForBranch(RDMABranch)
	if err != nil {
		return EvaluationHeads{}, err
	}
	firecracker, err := c.HeadForBranch(FirecrackerBranch)
	if err != nil {
		return EvaluationHeads{}, err
	}
	return EvaluationHeads{Khala: khala, InVitro: invitro, RDMA: rdma, Firecracker: firecracker}, nil
}

func ResolveEvaluationHeads(ctx context.Context, campaignPath string, smoke bool, setup Setup) (EvaluationHeads, error) {
	if !smoke {
		campaign, err := RequireCampaign(campaignPath)
		if err != nil {
			return EvaluationHeads{}, err
		}
		return campaign.EvaluationHeads()
	}
	invitro, err := GitProvenance(".")
	if err != nil || invitro.Branch != InVitroBranch {
		return EvaluationHeads{}, fmt.Errorf("smoke InVitro provenance: branch=%s: %w", invitro.Branch, err)
	}
	if err := invitro.ValidateClean(); err != nil {
		return EvaluationHeads{}, err
	}
	khala, err := GitProvenance("../khala")
	if err != nil || khala.Branch != KhalaBranch {
		return EvaluationHeads{}, fmt.Errorf("smoke Khala provenance: branch=%s: %w", khala.Branch, err)
	}
	if err := khala.ValidateClean(); err != nil {
		return EvaluationHeads{}, err
	}
	tenants := setup.LabeledIPs("minio-type=tenant")
	if len(tenants) != 1 {
		return EvaluationHeads{}, fmt.Errorf("smoke requires exactly one RDMA tenant")
	}
	target, err := setup.URLForIP(tenants[0])
	if err != nil {
		return EvaluationHeads{}, err
	}
	home, err := RemoteHome(target)
	if err != nil {
		return EvaluationHeads{}, err
	}
	command, err := SSHCommand(ctx, target, "git", "-C", home+"/rdma-demo", "rev-parse", "HEAD")
	if err != nil {
		return EvaluationHeads{}, err
	}
	output, err := command.CombinedOutput()
	if err != nil {
		return EvaluationHeads{}, fmt.Errorf("smoke RDMA provenance: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return EvaluationHeads{Khala: khala.Head, InVitro: invitro.Head, RDMA: strings.TrimSpace(string(output)), Firecracker: FirecrackerHead}, nil
}

func (h EvaluationHeads) Environment() []string {
	return []string{
		"EVAL_FIRECRACKER_HEAD=" + h.Firecracker,
		"EVAL_FIRECRACKER_BRANCH=" + FirecrackerBranch,
		"EVAL_RDMA_DEMO_HEAD=" + h.RDMA,
		"EVAL_RDMA_DEMO_BRANCH=" + RDMABranch,
		"EVAL_INVITRO_HEAD=" + h.InVitro,
		"EVAL_INVITRO_BRANCH=" + InVitroBranch,
		"EVAL_KHALA_HEAD=" + h.Khala,
		"EVAL_KHALA_BRANCH=" + KhalaBranch,
	}
}
