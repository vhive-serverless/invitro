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

	/**
	 * Limit the number of invocations per minute based upon `rps`.
	 * TODO: Extract out.
	 * */
	requestedInvocation := sum(totalInvocationsEachMin)
	requestLimit := rps * 60
	log.Info("Maximum #requests/min: ", requestLimit)
	for min, minuteInvocations := range totalInvocationsEachMin {
		if minuteInvocations > requestLimit {
			log.Warnf("Total number requests exceed capacity by %d at minute %d/%d",
				minuteInvocations-requestLimit, min+1, len(totalInvocationsEachMin))

			totalInvocationsEachMin[min] = requestLimit
		}
	}
	totalInvocaked := 0

	for min := 0; min < len(totalInvocationsEachMin); min++ {
		tolerance := time.Second * 2 // ! Tolerate 2s difference.
		timeout := time.After(time.Duration(60)*time.Second + tolerance)
		tick := time.NewTicker(time.Duration(1000/rps) * time.Millisecond).C
		start := time.Now()

		invocationCount := 0
		totalInvocationsThisMinute := totalInvocationsEachMin[min]
		// totalInvocationsThisMinute := 1 // ! Test memory over-commission.

		var oneMinuteInMicrosec int = 60e6
		funcSlot := time.Duration(int64(oneMinuteInMicrosec/totalInvocationsThisMinute)) * time.Microsecond
		log.Info("(Minute", min+1, ") Slot duration: ", funcSlot)

		wg := sync.WaitGroup{}

	this_minute:
		// Only invoke the number of functions limited by `rps`.
		for i := 0; i < totalInvocationsThisMinute; i++ {
			// TODO: Bulk the computation and move it out
			shuffleInvocationOfOneMin(&invocationsPerMin[min])

			functionIdx := invocationsPerMin[min][i]
			function := functions[functionIdx]
			if !function.GetStatus() {
				// Failed in deployment.
				continue
			}
			go func() {
				// 	select {
				// 	case <-ctx.Done():
				// 		log.Warn("TIME OUT when invoking ", function.GetName())
				// 		break
				// 	default:
				// 		goto Invocation
				// 	}
				// Invocation:
				var (
					hasInvoked bool
					latency    int64
				)
				defer func() {
					offset := funcSlot - time.Duration(latency)
					log.Info("Offset of this invocation: ", offset)
					if offset > time.Duration(0) {
						time.Sleep(offset)
					}
					wg.Done()
				}()

				wg.Add(1)
				// Bound invocation.
				ctx, cancel := context.WithTimeout(context.Background(), funcSlot)
				defer cancel()
				hasInvoked, latency = invoke(ctx, function)

				if hasInvoked {
					invocationCount++
				} else {
					return
				}
			}()
			select {
			case <-timeout:
				log.Warn("TIME OUT during invocation ", i, " round ", min)
				break this_minute
			case <-tick:
				duration := time.Since(start).Seconds()
				log.Info("One-minute excution duration: ", duration, " [s]")
				continue
			}
		}
		wg.Wait()

		totalInvocaked += invocationCount
		// TODO: Move IO elsewhere.
		if _, err := f.Write(
			[]byte(
				fmt.Sprintf("Requested %d invocations at min %d\n", totalInvocationsEachMin[min], min+1) +
					fmt.Sprintf("Issued    %d invocations at min %d\n\n", invocationCount, min+1))); err != nil {
			log.Fatal(err)
		}
	}
	// Wait for all -- don't wait on a min-by-min basis.
	if _, err := f.Write(
		[]byte(
			fmt.Sprintf("Original requested: %d. Actually invoked: %d\n",
				requestedInvocation, totalInvocaked))); err != nil {
		log.Fatal(err)
	}

	if err := f.Close(); err != nil {
		log.Fatal(err)
	}
}

func invoke(ctx context.Context, function tc.Function) (bool, int64) {
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
		return false, 0
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
		return false, 0
	}

	latency := time.Since(start).Microseconds()

	log.Infof("Invoked %s in %d [µs]\n", function.GetName(), latency)
	return true, latency
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
