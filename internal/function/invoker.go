package function

import (
	"context"
	_ "fmt"
	_ "strconv"
	"sync"
	_ "sync/atomic"
	"time"

	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"

	faas "github.com/eth-easl/easyloader/pkg/faas"
)

func Invoke(endpoints []string) {
	runDuration := 5              // TODO: Constant duration.
	timeout := 1000 * time.Second // TODO: Inifinite wait.

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	wg := sync.WaitGroup{}
	for i, endpoint := range endpoints {
		wg.Add(1)

		go func(i int, endpoint string) {
			defer wg.Done()
			invoke(ctx, endpoint, runDuration)
		}(i, endpoint)
	}
	wg.Wait()
}

func invoke(ctx context.Context, endpoint string, runDuration int) {
	// Start latency measurement.
	start := time.Now()

	conn, err := grpc.DialContext(ctx, endpoint, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()

	c := faas.NewExecutorClient(conn)

	// Contact the server and print out its response.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, err = c.Execute(ctx, &faas.FaasRequest{
		Input: "", Runtime: uint32(runDuration), Memory: uint32(0)})

	if err != nil {
		log.Warnf("Failed to invoke %s, err=%v", endpoint, err)
	}

	latency := time.Since(start).Microseconds()

	log.Infof("Invoked %s in %d [microsec]\n", endpoint, latency)

}
