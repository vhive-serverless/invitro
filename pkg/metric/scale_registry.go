package metric

import (
	util "github.com/eth-easl/loader/pkg"
	tc "github.com/eth-easl/loader/pkg/trace"
)

type ScaleRegistry struct {
	scaleCounter map[string]int
}

func (r *ScaleRegistry) Init(functions []tc.Function) {
	r.scaleCounter = map[string]int{}
	for _, f := range functions {
		r.scaleCounter[f.Name] = 0
	}
}

//! Since all functions are deployed once, we assume no duplications.
func (r *ScaleRegistry) UpdateAndGetColdStartCount(records []DeploymentScale) int {
	coldStarts := 0
	for _, record := range records {
		prevScale := r.scaleCounter[record.Deployment]
		currScale := record.Scale

		//* Check if it's scaling from 0.
		if prevScale == 0 {
			coldStarts += util.MaxOf(currScale, 0)
		}
		//* Update registry
		r.scaleCounter[record.Deployment] = currScale
	}
	return coldStarts
}
