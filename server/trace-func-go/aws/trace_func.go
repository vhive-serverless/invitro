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

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/vhive-serverless/loader/pkg/workload/standard"
	"time"
)

// Response is of type APIGatewayProxyResponse since we're leveraging the
// AWS Lambda Proxy Request functionality (default behavior)
//
// https://serverless.com/framework/docs/providers/aws/events/apigateway/#lambda-proxy-integration
type Response events.APIGatewayProxyResponse

// Handler is our lambda handler invoked by the `lambda.Start` function call
func Handler(_ context.Context, event events.LambdaFunctionURLRequest) (Response, error) {
	var buf bytes.Buffer

	start := time.Now()

	// Obtain payload from the request
	var req struct {
		RuntimeInMilliSec uint32 `json:"RuntimeInMilliSec"`
		MemoryInMebiBytes uint32 `json:"MemoryInMebiBytes"`
	}

	err := json.Unmarshal([]byte(event.Body), &req)
	if err != nil {
		return Response{StatusCode: 400}, err
	}

	standard.IterationsMultiplier = 102 // Cloudlab xl170 benchmark @ 1 second function execution time
	_ = standard.TraceFunctionExecution(start, req.RuntimeInMilliSec)

	body, err := json.Marshal(map[string]interface{}{
		"DurationInMicroSec": uint32(time.Since(start).Microseconds()),
		"MemoryUsageInKb":    req.MemoryInMebiBytes * 1024,
	})
	if err != nil {
		return Response{StatusCode: 400}, err
	}
	json.HTMLEscape(&buf, body)

	resp := Response{
		StatusCode:      200,
		IsBase64Encoded: false,
		Body:            buf.String(),
		Headers: map[string]string{
			"Content-Type":           "application/json",
			"X-MyCompany-Func-Reply": "trace_func_go handler",
		},
	}

	return resp, nil
}

func main() {
	lambda.Start(Handler) // Uses HTTP server under the hood
}
