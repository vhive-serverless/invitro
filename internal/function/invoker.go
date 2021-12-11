package function

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	_ "strconv"
	"sync"
	_ "sync/atomic"
	"time"

	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"

	tc "github.com/eth-easl/easyloader/internal/trace"
	faas "github.com/eth-easl/easyloader/pkg/faas"
)

func Invoke(
	rps int,
	functions []tc.Function,
	invocationsPerMin [][]int,
	totalInvocationsEachMin []int) {

	timeout := 1000 * time.Hour // TODO: Inifinite wait.

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	for min := 0; min < len(totalInvocationsEachMin); min++ {
		requestLimit := rps * 60
		totalInvocation := totalInvocationsEachMin[min]

		invocationCount := 0

		if totalInvocation > requestLimit {
			log.Warn("Total number requests exceed capacity at minute: ", min+1,
				" by ", totalInvocation-requestLimit)

			totalInvocation = requestLimit
		}

		wg := sync.WaitGroup{}
		wg.Add(totalInvocation)

		interval := time.Duration(60*1e6/totalInvocation) * time.Microsecond

		for i := 0; i < totalInvocation; i++ {
			shuffleInvocationOfOneMin(&invocationsPerMin[min])

			functionIdx := invocationsPerMin[min][i]
			function := functions[functionIdx]
			if !function.GetStatus() {
				// Failed in deployment.
				continue
			}

			go func(i int, endpoint string) {
				defer wg.Done()
				invoke(ctx, function)
				invocationCount++
			}(i, function.GetUrl())

			time.Sleep(interval)
		}
		wg.Wait()
		// If the file doesn't exist, create it, or append to the file.
		f, err := os.OpenFile("invocation.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			log.Fatal(err)
		}

		if _, err := f.Write(
			[]byte(
				fmt.Sprintf("Requested %d invocations at min %d\n", totalInvocationsEachMin[min], min+1) +
					fmt.Sprintf("Issued    %d invocations at min %d\n\n", invocationCount, min+1))); err != nil {
			log.Fatal(err)
		}
		if err := f.Close(); err != nil {
			log.Fatal(err)
		}
	}
}

func invoke(ctx context.Context, function tc.Function) {
	runtime, _ := tc.GetExecutionSpecification(function)
	// ! Memory allocations over-committed the server, which caused pods constantly fail
	// ! and be brought back to life again.
	memory := 10
	// Start latency measurement.
	start := time.Now()

	conn, err := grpc.DialContext(ctx, function.GetUrl(), grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()

	c := faas.NewExecutorClient(conn)

	// Contact the server and print out its response.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, err = c.Execute(ctx, &faas.FaasRequest{
		Input: "", Runtime: uint32(runtime), Memory: uint32(memory)})

	if err != nil {
		log.Warnf("Failed to invoke %s, err=%v", function.GetName(), err)
	}

	latency := time.Since(start).Microseconds()

	log.Infof("Invoked %s in %d [µs]\n", function.GetName(), latency)

}

/**
 * This function has/uses side-effects, but for the sake of performance
 * keep it for now.
 */
func shuffleInvocationOfOneMin(invocations *[]int) {
	for i := range *invocations {
		j := rand.Intn(i + 1)
		(*invocations)[i], (*invocations)[j] = (*invocations)[j], (*invocations)[i]
	}
}
