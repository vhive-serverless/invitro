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

type ScaleRegistry struct {
	scaleGauge map[string]int
}

func (r *ScaleRegistry) Init(records []ScaleRecord) {
	r.scaleGauge = map[string]int{}
	for _, record := range records {
		r.scaleGauge[record.Deployment] = record.ActualScale
	}
}

// ! Since all functions are deployed once, we assume no duplications.
func (r *ScaleRegistry) UpdateAndGetColdStartCount(records []ScaleRecord) int {
	coldStarts := 0
	for _, record := range records {
		prevScale := r.scaleGauge[record.Deployment]
		currScale := record.ActualScale

		//* Check if it's scaling from 0.
		if prevScale == 0 && currScale > 0 {
			coldStarts++
		}
		//* Update registry.
		r.scaleGauge[record.Deployment] = currScale
	}
	return coldStarts
}

func (r *ScaleRegistry) GetOneColdFunctionName() string {
	for f, scale := range r.scaleGauge {
		if scale == 0 {
			return f
		}
	}
	return "None"
}
