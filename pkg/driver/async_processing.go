package driver

import (
	"bytes"
	log "github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/pkg/driver/clients"
	"github.com/vhive-serverless/loader/pkg/metric"
	"io"
	"math"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

func (d *Driver) writeAsyncRecordsToLog(logCh chan *metric.ExecutionRecord) {
	const batchSize = 50

	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout: 2 * time.Second,
			}).DialContext,
			IdleConnTimeout:     time.Second,
			MaxIdleConns:        batchSize,
			MaxIdleConnsPerHost: batchSize,
		},
	}

	currentBatch := 0
	totalBatches := int(math.Ceil(float64(d.AsyncRecords.Length()) / float64(batchSize)))

	log.Infof("Gathering functions responses...")
	for d.AsyncRecords.Length() > 0 {
		currentBatch++

		toProcess := batchSize
		if d.AsyncRecords.Length() < batchSize {
			toProcess = d.AsyncRecords.Length()
		}

		wg := sync.WaitGroup{}
		wg.Add(toProcess)

		for i := 0; i < toProcess; i++ {
			go func() {
				defer wg.Done()

				start := time.Now()

				record := d.AsyncRecords.Dequeue()
				response, e2e := d.getAsyncResponseData(
					client,
					d.Configuration.DirigentConfiguration.AsyncResponseURL,
					record.AsyncResponseID,
				)

				if string(response) != "" {
					err := clients.DeserializeDirigentResponse(response, record)
					if err != nil {
						log.Errorf("Failed to deserialize Dirigent response - %v - %v", string(response), err)
					}
				} else {
					record.FunctionTimeout = true
					record.AsyncResponseID = ""
					log.Errorf("Failed to fetch response. The function has probably not yet completed.")
				}

				// loader send request + request e2e + loader get response
				timeToFetchResponse := time.Since(start).Microseconds()
				record.UserCodeExecutionMs = int64(e2e)
				record.TimeToGetResponseMs = timeToFetchResponse
				record.ResponseTime += int64(e2e)
				record.ResponseTime += timeToFetchResponse

				logCh <- record
			}()
		}

		wg.Wait()

		log.Infof("Processed %d/%d batches of async response gatherings", currentBatch, totalBatches)
	}

	log.Infof("Finished gathering async reponse answers")
}

func (d *Driver) getAsyncResponseData(client *http.Client, endpoint string, guid string) ([]byte, int) {
	req, err := http.NewRequest("GET", "http://"+endpoint, bytes.NewReader([]byte(guid)))
	if err != nil {
		log.Errorf("Failed to retrieve Dirigent response for %s - %v", guid, err)
		return []byte{}, 0
	}

	// TODO: set function name for load-balancing purpose
	//req.Header.Set("function", function.Name)

	resp, err := client.Do(req)
	if err != nil {
		log.Errorf("Failed to retrieve Dirigent response for %s - %v", guid, err)
		return []byte{}, 0
	}

	defer clients.HandleBodyClosing(resp)
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Errorf("Failed to read Dirigent response body for %s - %v", guid, err)
		return []byte{}, 0
	}

	hdr := resp.Header.Get("Duration-Microseconds")
	e2e := 0

	if hdr != "" {
		e2e, err = strconv.Atoi(hdr)
		if err != nil {
			log.Errorf("Failed to parse end-to-end latency for %s - %v", guid, err)
		}
	}

	return body, e2e
}
