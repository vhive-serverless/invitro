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

package main

import "C"
import (
	"encoding/json"
	"strconv"
	"time"
)

const ExecUnit int = 1e2

func takeSqrts() C.double {
	var tmp C.double // Circumvent compiler optimizations
	for i := 0; i < ExecUnit; i++ {
		tmp = C.SQRTSD(C.double(10))
	}
	return tmp
}

func busySpin(multiplier, runtimeMilli uint32) {
	totalIterations := int(multiplier * runtimeMilli)

	for i := 0; i < totalIterations; i++ {
		takeSqrts()
	}
}

type FunctionResponse struct {
	Status        string `json:"Status"`
	Function      string `json:"Function"`
	MachineName   string `json:"MachineName"`
	ExecutionTime int64  `json:"ExecutionTime"`
}

func Main(obj map[string]interface{}) map[string]interface{} {
	function := obj["function"].(string)
	requestedCpu := obj["requested_cpu"].(string)
	multiplier := obj["multiplier"].(string)

	result := make(map[string]interface{})

	tlm, err := strconv.Atoi(requestedCpu)
	if err != nil {
		return result
	}

	mpl, err := strconv.Atoi(multiplier)
	if err != nil {
		return result
	}

	start := time.Now()
	timeLeftMilliseconds := uint32(tlm)

	timeConsumedMilliseconds := uint32(time.Since(start).Milliseconds())
	if timeConsumedMilliseconds < timeLeftMilliseconds {
		timeLeftMilliseconds -= timeConsumedMilliseconds
		if timeLeftMilliseconds > 0 {
			busySpin(uint32(mpl), timeLeftMilliseconds)
		}
	}

	responseBytes, _ := json.Marshal(FunctionResponse{
		Status:        "OK",
		Function:      function,
		MachineName:   "NYI",
		ExecutionTime: time.Since(start).Microseconds(),
	})

	result["body"] = responseBytes

	return result
}
