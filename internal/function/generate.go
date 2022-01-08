package function

import (
	"context"
	"math/rand"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gocarina/gocsv" // Use `csv:-`` to ignore a field.
	log "github.com/sirupsen/logrus"

	util "github.com/eth-easl/loader/internal"
	tc "github.com/eth-easl/loader/internal/trace"
)

func Generate(
	rps int,
	functions []tc.Function,
	invocationsEachMinute [][]int,
	totalNumInvocationsEachMinute []int) {

	totalDurationMinutes := len(totalNumInvocationsEachMinute)
	start := time.Now()
	wg := sync.WaitGroup{}

	invocRecords := []*tc.MinuteInvocationRecord{}
	latencyRecords := []*tc.LatencyRecord{}
	idleDuration := time.Duration(0)

	for minute := 0; minute < int(totalDurationMinutes); minute++ {
		if rps < 1 {
			//* If `rps` is not specified, we distribute invocations uniformly for now.
			//TODO: Implement Poisson.
			rps = totalNumInvocationsEachMinute[minute]/60 + 1
		}
		log.Info("RPS: ", rps)

		numFuncInvocaked := 0
		idleDuration = time.Duration(0)
		//TODO: Bulk the computation and move it out
		shuffleInplaceInvocationOfOneMinute(&invocationsEachMinute[minute])

		var record tc.MinuteInvocationRecord
		record.MinuteIdx = minute
		record.Rps = rps

		/** Set up timer to bound the one-minute invocation. */
		iterStart := time.Now()
		epsilon := time.Duration(0)
		timeout := time.After(time.Duration(60)*time.Second - epsilon)
		interval := time.Duration(1000/rps) * time.Millisecond
		ticker := time.NewTicker(interval)
		done := make(chan bool)

		/** Launch a timer. */
		go func() {
			t := <-timeout
			log.Warn("(Slot finished)\t", t.Format(time.StampMilli), "\tMinute Nbr. ", minute)
			ticker.Stop()
			done <- true
		}()

		//* Bound the #invocations by `rps`.
		numFuncToInvokeThisMinute := util.MinOf(rps*60, totalNumInvocationsEachMinute[minute])
		var invocationCount int32

		next := 0
		for {
			select {
			case t := <-ticker.C:
				if next >= numFuncToInvokeThisMinute {
					log.Info("Idle ticking at ", t.Format(time.StampMilli), "\tMinute Nbr. ", minute, " Itr. ", next)
					idleDuration += interval
					continue
				}
				go func(m int, nxt int) {
					defer wg.Done()
					wg.Add(1)
					//TODO: Make Dialling timeout customisable.
					diallingBound := 2 * time.Minute //* 2-min timeout for circumventing hanging.
					ctx, cancel := context.WithTimeout(context.Background(), diallingBound)
					defer cancel()

					funcIndx := invocationsEachMinute[m][nxt]
					function := functions[funcIndx]
					hasInvoked, latencyRecord := invoke(ctx, function)
					latencyRecord.Rps = rps

					if hasInvoked {
						atomic.AddInt32(&invocationCount, 1)
						latencyRecords = append(latencyRecords, &latencyRecord)
					}
				}(minute, next)
			case <-done:
				numFuncInvocaked += int(invocationCount)
				log.Info("Iteration spent: ", time.Since(iterStart), "\tMinute Nbr. ", minute)
				log.Info("Required #invocations=", totalNumInvocationsEachMinute[minute],
					" Fired #functions=", numFuncInvocaked, "\tMinute Nbr. ", minute)

				record.Duration = time.Since(iterStart).Microseconds()
				record.IdleDuration = idleDuration.Microseconds()
				record.NumFuncRequested = totalNumInvocationsEachMinute[minute]
				record.NumFuncInvoked = numFuncInvocaked
				record.NumFuncFailed = numFuncToInvokeThisMinute - numFuncInvocaked
				invocRecords = append(invocRecords, &record)
				goto next_minute //* `break` doesn't work here as it's somehow ambiguous to Golang.
			}
			next++
		}
	next_minute:
	}
	//! Hyperparameter for busy-wait (currently it's the same duration as that of the traces).
	//TODO: Make this force wait customisable.
	log.Info("\tFinished invoking all functions.\n\tStart waiting for all requests to return.")
	delta := time.Duration(1)
	//! Force timeout as the last resort.
	forceTimeout := time.Duration(totalDurationMinutes) * time.Minute / delta
	if wgWaitWithTimeout(&wg, forceTimeout) {
		log.Warn("Time out waiting for fired invocations to return.")
	} else {
		totalDuration := time.Since(start)
		log.Info("Total invocation + waiting duration: ", totalDuration, "\tIdle ", idleDuration, "\n")
	}

	//TODO: Extract IO out.
	invocFileName := "data/out/invoke_rps-" + strconv.Itoa(rps) + "_dur-" + strconv.Itoa(len(invocRecords)) + ".csv"
	invocF, err := os.Create(invocFileName)
	util.Check(err)
	gocsv.MarshalFile(&invocRecords, invocF)

	latencyFileName := "data/out/latency_rps-" + strconv.Itoa(rps) + "_dur-" + strconv.Itoa(len(invocRecords)) + ".csv"
	latencyF, err := os.Create(latencyFileName)
	util.Check(err)
	gocsv.MarshalFile(&latencyRecords, latencyF)
}

/**
 * This function waits for the waitgroup for the specified max timeout.
 * Returns true if waiting timed out.
 */
func wgWaitWithTimeout(wg *sync.WaitGroup, timeout time.Duration) bool {
	c := make(chan struct{})
	go func() {
		defer close(c)
		wg.Wait()
	}()
	select {
	case <-c:
		log.Info("Busy waiting finished at the end of all invocations.")
		return false
	case <-time.After(timeout):
		return true
	}
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
