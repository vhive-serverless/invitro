package clients

import (
	"bytes"
	"errors"
	"fmt"
	log "github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/pkg/common"
	"github.com/vhive-serverless/loader/pkg/metric"
	"go.mongodb.org/mongo-driver/bson"
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

func composeDandelionMatMulBody(functionName string) *bytes.Buffer {
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
		log.Debugf("Error encoding Dandelion MatMul request - %v", err)
		return nil
	}

	return bytes.NewBuffer(body)
}

func DeserializeDandelionResponse(function *common.Function, body []byte, record *metric.ExecutionRecord) error {
	var result DandelionDeserializeResponse
	err := bson.Unmarshal(body, &result)
	if err != nil {
		return errors.New(fmt.Sprintf("Error deserializing response body - %v", err))
	}

	if len(result.Sets) != 1 {
		return errors.New("error: Unexpected sets length")
	} else if len(result.Sets[0].Items) != 1 {
		return errors.New("error: Unexpected sets[0].items length")
	}

	responseData := result.Sets[0].Items[0].Data
	if len(responseData) != 16 {
		return errors.New("error: unexpected responseData length")
	}

	record.Instance = function.Name
	record.ActualDuration = 0 // this field is not used yet in benchmark

	return nil
}
