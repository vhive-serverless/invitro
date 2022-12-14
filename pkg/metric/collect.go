package metric

import (
	"encoding/json"
	"os/exec"
	"time"

	log "github.com/sirupsen/logrus"
)

/*func (collector *Collector) GetOneColdStartFunction() common.Function {
	funcName := collector.scaleRegistry.GetOneColdFunctionName()
	return common.Function{
		Name:     funcName,
		Endpoint: tc.GetFuncEndpoint(funcName),
	}
}*/

func ScrapeDeploymentScales() []ScaleRecord {
	cmd := exec.Command(
		"python3",
		"pkg/metric/scrape_scales.py",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Warn("Fail to scrape deployment scales: ", err)
	}

	var results []DeploymentScale
	err = json.Unmarshal(out, &results)
	if err != nil {
		log.Warn("Fail to parse deployment scales: ", string(out[:]), err)
	}

	timestamp := time.Now().UnixMicro()
	records := []ScaleRecord{}
	for _, result := range results {
		records = append(records, ScaleRecord{
			Timestamp:    timestamp,
			Deployment:   result.Deployment,
			DesiredScale: result.DesiredScale,
			ActualScale:  result.ActualScale,
		})
	}
	return records
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
