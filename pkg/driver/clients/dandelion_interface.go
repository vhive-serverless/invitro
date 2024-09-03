package clients

import (
	"bytes"
	"fmt"
	"github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/pkg/common"
	"github.com/vhive-serverless/loader/pkg/metric"
	"go.mongodb.org/mongo-driver/bson"
	"strings"
)

type InputItem struct {
	Identifier string `bson:"identifier"`
	Key        int64  `bson:"key"`
	Data       []byte `bson:"data"`
}

type InputSet struct {
	Identifier string      `bson:"identifier"`
	Items      []InputItem `bson:"items"`
}

type DandelionRequest struct {
	Name string     `bson:"name"`
	Sets []InputSet `bson:"sets"`
}

type DandelionDeserializeResponse struct {
	Sets []InputSet `bson:"sets"`
}

/*func composeDandelionMatMulBody(functionName string) *bytes.Buffer {
	request := DandelionRequest{
		Name: functionName,
		Sets: []InputSet{
			{
				Identifier: "",
				Items: []InputItem{
					{
						Identifier: "",
						Key:        0,
						Data:       []byte{1, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0},
					},
				},
			},
		},
	}
	body, err := bson.Marshal(request)
	if err != nil {
		logrus.Debugf("Error encoding Dandelion MatMul request - %v", err)
		return nil
	}
	return bytes.NewBuffer(body)
}*/

func composeBusyLoopBody(functionName, image string, runtime, iterations int) *bytes.Buffer {
	request := DandelionRequest{
		Name: functionName,
		Sets: []InputSet{
			{
				Identifier: "",
				Items: []InputItem{
					{
						Identifier: "input.csv",
						Key:        0,
						Data: []byte(fmt.Sprintf(
							"%s,%s,%d,%d",
							functionName,
							image,
							// TODO: bug in the current image '\0'. Remove '* 10' when new image is applied
							runtime*10,
							iterations,
						)),
					},
				},
			},
		},
	}

	body, err := bson.Marshal(request)
	if err != nil {
		logrus.Debugf("Error encoding Dandelion MatMul request - %v", err)
		return nil
	}

	return bytes.NewBuffer(body)
}

func DeserializeDandelionResponse(function *common.Function, body []byte, record *metric.ExecutionRecord) error {
	var result DandelionDeserializeResponse
	err := bson.Unmarshal(body, &result)
	if err != nil {
		return fmt.Errorf("error deserializing response body - %v", err)
	}

	rawResponseData := result.Sets[0].Items[0].Data
	data := strings.Split(string(rawResponseData), ",")

	if len(data) > 0 && !strings.Contains(strings.ToLower(data[0]), "ok") {
		record.FunctionTimeout = false
	}

	record.Instance = function.Name
	record.ActualDuration = 0 // this field is not used yet in benchmark

	return nil
}
