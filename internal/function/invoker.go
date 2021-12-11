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

	// If the file doesn't exist, create it, or append to the file.
	f, err := os.OpenFile("invocation.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatal(err)
	}

	timeout := 1000 * time.Hour // TODO: Inifinite wait.

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	/** Limit the number of invocations per minute based upon `rps`. */
	requestLimit := rps * 60
	for min, minuteInvocations := range totalInvocationsEachMin {
		if minuteInvocations > requestLimit {
			log.Warn("Total number requests exceed capacity at minute: ", min+1,
				" by ", minuteInvocations-requestLimit)

			totalInvocationsEachMin[min] = requestLimit
		}
	}
	totalInvocations := sum(totalInvocationsEachMin)
	totalInvocaked := 0

	wg := sync.WaitGroup{}
	for min := 0; min < len(totalInvocationsEachMin); min++ {
		invocationCount := 0
		totalInvocationsThisMin := totalInvocationsEachMin[min]
		// totalInvocationThisMin := 1 // ! Test memory over-commission.

		oneMinuteInSec := 60
		invocationOffset := time.Duration(2e6) // ! This is a rough estimation and is thus to be improved.
		interval := time.Duration(oneMinuteInSec/totalInvocationsThisMin)*time.Microsecond - invocationOffset

		// Only invoke the number of functions limited by `rps`.
		for i := 0; i < totalInvocationsThisMin; i++ {
			shuffleInvocationOfOneMin(&invocationsPerMin[min])

			functionIdx := invocationsPerMin[min][i]
			function := functions[functionIdx]
			if !function.GetStatus() {
				// Failed in deployment.
				continue
			}
			wg.Add(1)
			go func(i int, endpoint string) {
				defer wg.Done()
				if invoke(ctx, function) {
					invocationCount++
				} else {
					return
				}
			}(i, function.GetUrl())

			time.Sleep(interval)
		}
		// TODO: Move IO elsewhere.
		if _, err := f.Write(
			[]byte(
				fmt.Sprintf("Requested %d invocations at min %d\n", totalInvocationsEachMin[min], min+1) +
					fmt.Sprintf("Issued    %d invocations at min %d\n\n", invocationCount, min+1))); err != nil {
			log.Fatal(err)
		}

		totalInvocaked += invocationCount
	}
	// Wait for all -- don't wait on a min-by-min basis.
	wg.Wait()
	if _, err := f.Write(
		[]byte(
			fmt.Sprintf("Total requested: %d. Actually invoked: %d\n", totalInvocations, totalInvocaked))); err != nil {
		log.Fatal(err)
	}

	if err := f.Close(); err != nil {
		log.Fatal(err)
	}
}

func invoke(ctx context.Context, function tc.Function) bool {
	runtime, _ := tc.GetExecutionSpecification(function)
	// ! * Memory allocations over-committed the server, which caused pods constantly fail
	// ! and be brought back to life again.
	// ! * Set to 1 MB for testing purposes.
	memory := 1

	// Start latency measurement.
	start := time.Now()

	conn, err := grpc.DialContext(ctx, function.GetUrl(), grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
		return false
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
		return false
	}

	latency := time.Since(start).Microseconds()

	log.Infof("Invoked %s in %d [µs]\n", function.GetName(), latency)
	return true
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

func sum(array []int) int {
	result := 0
	for _, v := range array {
		result += v
	}
	return result
}
