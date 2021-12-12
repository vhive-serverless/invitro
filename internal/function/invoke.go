package function

import (
	"context"
	"math/rand"
	"os"
	"strconv"
	"sync"
	_ "sync/atomic"
	"time"

	"google.golang.org/grpc"

	"github.com/gocarina/gocsv" // Use `csv:-`` to ignore a field.
	log "github.com/sirupsen/logrus"

	tc "github.com/eth-easl/easyloader/internal/trace"
	faas "github.com/eth-easl/easyloader/pkg/faas"
)

func Invoke(
	rps int,
	functions []tc.Function,
	invocationsEachMinute [][]int,
	totalNumInvocationsEachMinute []int) {

	start := time.Now()
	wg := sync.WaitGroup{}

	invocRecords := []*tc.MinuteInvocationRecord{}
	latencyRecords := []*tc.LatencyRecord{}
	idleDuration := time.Duration(0)

	for minute := 0; minute < len(totalNumInvocationsEachMinute); minute++ {
		numFuncInvocaked := 0
		idleDuration = time.Duration(0)
		//TODO: Bulk the computation and move it out
		shuffleInplaceInvocationOfOneMinute(&invocationsEachMinute[minute])

		var record tc.MinuteInvocationRecord
		record.Rps = rps
		record.MinuteIdx = minute
		iter_start := time.Now()

		/** Set up timer to bound the one-minute invocation. */
		epsilon := time.Duration(0)
		timeout := time.After(time.Duration(60)*time.Second - epsilon)
		interval := time.Duration(1000/rps) * time.Millisecond
		ticker := time.NewTicker(interval)
		done := make(chan bool)

		/** Timer. */
		go func() {
			t := <-timeout
			log.Warn("(regular) TIMEOUT at ", t, "\tMinute Nbr. ", minute)
			ticker.Stop()
			done <- true
		}()

		//! Bound the #invocations by `rps`.
		numFuncToInvokeThisMinute := MinOf(rps*60, totalNumInvocationsEachMinute[minute])
		invocationCount := 0

		next := 0
		for {
			select {
			case t := <-ticker.C:
				if next >= numFuncToInvokeThisMinute {
					log.Info("Idle ticking at ", t.Format(time.StampMilli), "\tMinute Nbr. ", minute, " Itr. ", next)
					idleDuration += interval
					continue
				}
				//TODO: Enable cucurrent invocations.
				go func() {
					defer wg.Done()
					wg.Add(1)

					diallingBound := 2 * time.Minute //* 2-min timeout to circumvent hanging.
					ctx, cancel := context.WithTimeout(context.Background(), diallingBound)
					defer cancel()

					funcIndx := invocationsEachMinute[minute][next]
					function := functions[funcIndx]
					hasInvoked, latencyRecord := invoke(ctx, function)
					latencyRecord.Rps = rps

					if hasInvoked {
						invocationCount++
						latencyRecords = append(latencyRecords, &latencyRecord)
					}
				}()
			case <-done:
				numFuncInvocaked += invocationCount
				log.Info("Iteration spent: ", time.Since(iter_start), "\tMinute Nbr. ", minute)
				log.Info("Required #invocations=", totalNumInvocationsEachMinute[minute],
					" Fired #functions=", numFuncInvocaked, "\tMinute Nbr. ", minute)

				record.Duration = time.Since(iter_start).Microseconds()
				record.IdleDuration = idleDuration.Microseconds()
				record.NumFuncRequested = totalNumInvocationsEachMinute[minute]
				record.NumFuncInvoked = numFuncInvocaked
				record.NumFuncFailed = numFuncToInvokeThisMinute - numFuncInvocaked
				invocRecords = append(invocRecords, &record)
				goto next_minute // *`break` doesn't work here as it's somehow ambiguous to Golang.
			}
			next++
		}
	next_minute:
	}
	wg.Wait()
	totalDuration := time.Since(start)
	log.Info("Total invocation duration: ", totalDuration, "\tIdle ", idleDuration, "\n")

	//TODO: Extract IO out.
	invocFileName := "data/out/invoke_rps-" + strconv.Itoa(rps) + "dur-" + strconv.Itoa(len(invocRecords)) + ".csv"
	invocF, err := os.Create(invocFileName)
	check(err)
	gocsv.MarshalFile(&invocRecords, invocF)

	latencyFileName := "data/out/latency_rps-" + strconv.Itoa(rps) + "dur-" + strconv.Itoa(len(invocRecords)) + ".csv"
	latencyF, err := os.Create(latencyFileName)
	check(err)
	gocsv.MarshalFile(&latencyRecords, latencyF)
}

func invoke(ctx context.Context, function tc.Function) (bool, tc.LatencyRecord) {
	runtimeRequested, _ := tc.GetExecutionSpecification(function)
	//! * Memory allocations over-committed the server, which caused pods constantly fail
	//! and be brought back to life again.
	//! * Set to 1 MB for testing purposes.
	memory := 1

	var record tc.LatencyRecord
	record.FuncName = function.GetName()

	// Start latency measurement.
	start := time.Now()
	record.Timestamp = start.UnixMicro()

	conn, err := grpc.DialContext(ctx, function.GetUrl(), grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		log.Warnf("Failed to connect: %v", err)
		return false, tc.LatencyRecord{}
	}
	defer conn.Close()

	c := faas.NewExecutorClient(conn)

	// Contact the server and print out its response.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	response, err := c.Execute(ctx, &faas.FaasRequest{
		Input: "", Runtime: uint32(runtimeRequested), Memory: uint32(memory)})

	if err != nil {
		log.Warnf("Failed to invoke %s, err=%v", function.GetName(), err)
		return false, tc.LatencyRecord{}
	}

	//TODO: Improve the sever-side.
	// log.Info("gRPC response: ", reply.Response)
	runtime := response.Latency
	record.Runtime = runtime
	log.Info("(gRPC) Function execution time: ", runtime, " [µs]")

	latency := time.Since(start).Microseconds()
	record.Latency = latency
	log.Infof("Invoked %s in %d [µs]\n", function.GetName(), latency)

	return true, record
}

/**
 * This function has/uses side-effects, but for the sake of performance
 * keep it for now.
 */
func shuffleInplaceInvocationOfOneMinute(invocations *[]int) {
	for i := range *invocations {
		j := rand.Intn(i + 1)
		(*invocations)[i], (*invocations)[j] = (*invocations)[j], (*invocations)[i]
	}
}

func MinOf(vars ...int) int {
	min := vars[0]

	for _, i := range vars {
		if min > i {
			min = i
		}
	}

	return min
}

// func sum(array []int) int {
// 	result := 0
// 	for _, v := range array {
// 		result += v
// 	}
// 	return result
// }

func check(e error) {
	if e != nil {
		panic(e)
	}
}
