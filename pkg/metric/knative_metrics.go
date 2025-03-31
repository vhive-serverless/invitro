/*
 * MIT License
 *
 * Copyright (c) 2023 EASL and the vHive community
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 */

package metric

import (
	"encoding/json"
	"os/exec"

	log "github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/pkg/common"
)

func ScrapeDeploymentScales() []common.DeploymentScale {
	cmd := exec.Command("python3", "pkg/metric/scrape_scales.py")
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Warn("Fail to scrape deployment scales: ", err)
	}

	var results []common.DeploymentScale
	err = json.Unmarshal(out, &results)
	if err != nil {
		log.Warn("Fail to parse deployment scales: ", string(out[:]), err)
	}

	return results
}

func ScrapeKnStats() common.KnStats {
	cmd := exec.Command(
		"python3",
		"pkg/metric/scrape_kn.py",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Warn("Fail to scrape Knative: ", err)
	}

	var result common.KnStats
	err = json.Unmarshal(out, &result)
	if err != nil {
		log.Warn("Fail to parse Knative: ", string(out[:]), err)
	}

	return result
}

func ScrapeClusterUsage() common.ClusterUsage {
	cmd := exec.Command("python3", "pkg/metric/scrape_infra.py")
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Warn("Fail to scrape cluster usage: ", err)
	}

	var result common.ClusterUsage
	err = json.Unmarshal(out, &result)
	if err != nil {
		log.Warn("Fail to parse cluster usage: ", string(out[:]), err)
	}

	return result
}
