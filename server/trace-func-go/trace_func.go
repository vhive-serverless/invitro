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
	"github.com/eth-easl/loader/pkg/workload/standard"
	log "github.com/sirupsen/logrus"
	"os"
	"strconv"
)

var (
	zipkin = flag.String("zipkin", "http://zipkin.zipkin:9411/api/v2/spans", "zipkin url")
)

func main() {
	// For containers - port 80; for Firecracker - 50051.
	var serverPort = 80
	var functionType standard.FunctionType

	if len(os.Args) > 1 {
		serverPort, _ = strconv.Atoi(os.Args[1])

		switch os.Args[2] {
		case "EMPTY":
			functionType = standard.EmptyFunction
		default:
			functionType = standard.TraceFunction
		}

		log.Infof("Port: %d\n", serverPort)
		log.Infof("Function type: %s\n", os.Args[2])
	}

	standard.StartGRPCServer("", serverPort, functionType, *zipkin)
}
