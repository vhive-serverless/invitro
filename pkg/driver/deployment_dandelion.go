package driver

import (
	"bytes"
	"fmt"
	"github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/pkg/common"
	"go.mongodb.org/mongo-driver/bson"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"strings"
)

type RegisterFunction struct {
	Name        string  `bson:"name"`
	ContextSize uint64  `bson:"context_size"`
	EngineType  string  `bson:"engine_type"`
	Binary      []int32 `bson:"binary"`
}

func DeployFunctionsDandelion(controlPlaneAddress string, functions []*common.Function) {
	for i := 0; i < len(functions); i++ {
		function := functions[i]
		name := functions[i].Name
		// FIXME: hard-coded function binary
		matmulPath := "/home/sai/thesis/dandelion/machine_interface/tests/data/test_sysld_wasm_x86_64_matmul"
		binaryData, err := ioutil.ReadFile(matmulPath)
		if err != nil {
			fmt.Println("Error reading binary file:", err)
			return
		}
		intData := make([]int32, len(binaryData))
		for i := 0; i < len(binaryData); i++ {
			intData[i] = int32(binaryData[i])
		}
		registerRequest := RegisterFunction{
			Name:        name,
			ContextSize: 0x8020000,
			// Binary:      []int32{1, 2, 3, 5},
			Binary:     intData,
			EngineType: "RWasm", // 替换为实际的引擎类型
		}

		registerRequestBody, err := bson.Marshal(registerRequest)
		if err != nil {
			fmt.Println("Error encoding register request:", err)
			return
		}

		url := fmt.Sprintf("http://%s/dandelion", controlPlaneAddress)
		logrus.Debugf("dandelion request body = ", len(registerRequestBody))
		resp, err := http.Post(url, "application/json", bytes.NewBuffer(registerRequestBody))
		if err != nil {
			logrus.Errorf("failed to reigster function", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			logrus.Debugf("register function %s ok!", name)
		} else {
			logrus.Debugf("register function %s failed, error code = %v", name, resp.StatusCode)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			logrus.Error("Failed to read response body.")
			return
		}

		endpoints := strings.Split(string(body), ";")
		if len(endpoints) == 0 {
			logrus.Error("Function registration returned no data plane(s).")
			return
		}
		function.Endpoint = endpoints[rand.Intn(len(endpoints))]
	}
}
