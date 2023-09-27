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
