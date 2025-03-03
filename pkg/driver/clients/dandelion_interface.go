package clients

import (
	"bytes"
	"fmt"
	"github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/pkg/common"
	"github.com/vhive-serverless/loader/pkg/metric"
	"go.mongodb.org/mongo-driver/bson"
	"net/url"
	"os"
	"path/filepath"
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

func filenameFromPath(path string) string {
	ident := filepath.Base(path)
	if pos := strings.LastIndexByte(ident, '.'); pos != -1 {
		ident = ident[:pos]
	}
	return ident
}

func CreateDandelionRequest(serviceName string, dataItems [][]string) *DandelionRequest {
	logrus.Debugf("Creating dandelion request for '%s' with following data:", serviceName)
	sets := make([]InputSet, len(dataItems))
	for setIdx, set := range dataItems {
		items := make([]InputItem, len(set))
		for itmIdx, itm := range set {
			data := []byte(itm)
			ident := itm

			// special cases
			if itm == "" {
				data = []byte{}
				ident = "empty"
			} else {
				var err error
				if strings.HasPrefix(itm, "%path=") {
					logrus.Tracef("Detected local path for item %d in set %d (%s)", itmIdx, setIdx, itm)
					data, err = os.ReadFile(itm[6:])
					if err != nil {
						logrus.Fatalf("Failed to read file '%s': %v", itm[6:], err)
					}
					ident = filenameFromPath(itm[6:])
				}
			}

			items[itmIdx] = InputItem{
				Identifier: ident,
				Key:        int64(itmIdx),
				Data:       data,
			}
			logrus.Debugf(" set %d, item %d -> %s (size=%d)\n", setIdx, itmIdx, ident, len(data))
		}
		sets[setIdx] = InputSet{
			Identifier: filenameFromPath(items[0].Identifier),
			Items:      items,
		}
	}
	return &DandelionRequest{
		Name: serviceName,
		Sets: sets,
	}
}

func WorkflowInvocationBody(wfName string, inData *DandelionRequest) string {
	var wfInput []byte
	var err error
	if inData == nil {
		wfInput = []byte{}
	} else {
		wfInput, err = bson.Marshal(inData)
		if err != nil {
			logrus.Errorf("Error encoding input data - %v\n", err)
			return ""
		}
	}

	body := url.Values{
		"name":  {wfName},
		"input": {string(wfInput)},
	}
	return body.Encode()
}

func DeserializeDandelionResponse(function *common.Function, body []byte, record *metric.ExecutionRecord, allowEmptyResponse bool) error {
	var result DandelionDeserializeResponse
	err := bson.Unmarshal(body, &result)
	if err != nil {
		return fmt.Errorf("error deserializing response body - %v", err)
	}

	if len(result.Sets) > 0 && len(result.Sets[0].Items) > 0 {
		rawResponseData := result.Sets[0].Items[0].Data
		data := strings.Split(string(rawResponseData), ",")

		if len(data) > 0 && !strings.Contains(strings.ToLower(data[0]), "ok") {
			record.FunctionTimeout = false
		}
	} else {
		if allowEmptyResponse {
			record.FunctionTimeout = false
		}
	}

	record.Instance = function.Name
	record.ActualDuration = 0 // this field is not used yet in benchmark

	return nil
}
