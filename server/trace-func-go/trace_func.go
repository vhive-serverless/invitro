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
	"flag"
	"os"
	"strconv"

	log "github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/pkg/workload/standard"
)

var (
	zipkin = flag.String("zipkin", "http://zipkin.zipkin:9411/api/v2/spans", "zipkin url")
)

func main() {
	// For containers - port 80; for Firecracker - 50051.
	var serverPort = 80
	var functionType standard.FunctionType

	if _, ok := os.LookupEnv("FUNC_PORT_ENV"); ok {
		serverPort, _ = strconv.Atoi(os.Getenv("FUNC_PORT_ENV"))
	}

	if _, ok := os.LookupEnv("FUNC_TYPE_ENV"); ok {
		switch os.Getenv("FUNC_TYPE_ENV") {
		case "EMPTY":
			functionType = standard.EmptyFunction
		default:
			functionType = standard.TraceFunction
		}
	}

	log.Infof("Port: %d\n", serverPort)
	if functionType == standard.TraceFunction {
		log.Infof("Function type: TRACE\n")
	} else {
		log.Infof("Function type: EMPTY\n")
	}

	standard.StartGRPCServer("", serverPort, functionType, *zipkin)
}
