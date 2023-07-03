package metric

import (
	"encoding/json"
	"os/exec"

	log "github.com/sirupsen/logrus"
)

func ScrapeDeploymentScales() []DeploymentScale {
	cmd := exec.Command("python3", "pkg/metric/scrape_scales.py")
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Warn("Fail to scrape deployment scales: ", err)
	}

	var results []DeploymentScale
	err = json.Unmarshal(out, &results)
	if err != nil {
		log.Warn("Fail to parse deployment scales: ", string(out[:]), err)
	}

	return results
}

func ScrapeKnStats() KnStats {
	cmd := exec.Command(
		"python3",
		"pkg/metric/scrape_kn.py",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Warn("Fail to scrape Knative: ", err)
	}

	var result KnStats
	err = json.Unmarshal(out, &result)
	if err != nil {
		log.Warn("Fail to parse Knative: ", string(out[:]), err)
	}

	return result
}

func ScrapeClusterUsage() ClusterUsage {
	cmd := exec.Command("python3", "pkg/metric/scrape_infra.py")
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Warn("Fail to scrape cluster usage: ", err)
	}

	var result ClusterUsage
	err = json.Unmarshal(out, &result)
	if err != nil {
		log.Warn("Fail to parse cluster usage: ", string(out[:]), err)
	}

	return result
}
