package metric

import (
	tc "github.com/eth-easl/loader/pkg/trace"
)

type ScaleRegistry struct {
	scaleGauge map[string]int
}

func (r *ScaleRegistry) Init(functions []tc.Function) {
	r.scaleGauge = map[string]int{}
	for _, f := range functions {
		r.scaleGauge[f.Name] = 0
	}
}

//! Since all functions are deployed once, we assume no duplications.
func (r *ScaleRegistry) UpdateAndGetColdStartCount(records []DeploymentScale) int {
	coldStarts := 0
	for _, record := range records {
		prevScale := r.scaleGauge[record.Deployment]
		currScale := record.Scale

		//* Check if it's scaling from 0.
		if prevScale == 0 && currScale > 0 {
			coldStarts++
		}
		//* Update registry.
		r.scaleGauge[record.Deployment] = currScale
	}
	return coldStarts
}
