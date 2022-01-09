package function

import (
	"context"
	"math"
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
	phaseIdx int,
	phaseOffset int,
	withBlocking bool,
	rps int,
	functions []tc.Function,
	invocationsEachMinute [][]int,
	totalNumInvocationsEachMinute []int) {

	isFixedRate := true
	if rps < 1 {
		isFixedRate = false
	}

	totalDurationMinutes := len(totalNumInvocationsEachMinute)
	start := time.Now()
	idleDuration := time.Duration(0)
	wg := sync.WaitGroup{}

	invocRecords := []*tc.MinuteInvocationRecord{}
	latencyRecords := []*tc.LatencyRecord{}

	for minute := 0; minute < int(totalDurationMinutes); minute++ {
		if !isFixedRate {
			//* We distribute invocations uniformly for now.
			//TODO: Implement Poisson.
			rps = int(math.Ceil(float64(totalNumInvocationsEachMinute[minute]) / 60))
		}
		log.Infof("Minute[%d]\t RPS=%d", minute, rps)

		numFuncInvocaked := 0
		idleDuration = time.Duration(0)
		//TODO: Bulk the computation and move it out
		shuffleInplaceInvocationOfOneMinute(&invocationsEachMinute[minute])

		var invocRecord tc.MinuteInvocationRecord
		invocRecord.MinuteIdx = minute + phaseOffset
		invocRecord.Phase = phaseIdx
		invocRecord.Rps = rps

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
				go func(m int, nxt int, phase int, rps int) {
					defer wg.Done()
					wg.Add(1)
					//TODO: Make Dialling timeout customisable.
					diallingBound := 2 * time.Minute //* 2-min timeout for circumventing hanging.
					ctx, cancel := context.WithTimeout(context.Background(), diallingBound)
					defer cancel()

					funcIndx := invocationsEachMinute[m][nxt]
					function := functions[funcIndx]
					hasInvoked, latencyRecord := invoke(ctx, function)

					latencyRecord.Phase = phase
					latencyRecord.Rps = rps

					if hasInvoked {
						atomic.AddInt32(&invocationCount, 1)
						latencyRecords = append(latencyRecords, &latencyRecord)
					}
				}(minute, next, phaseIdx, rps)
			case <-done:
				numFuncInvocaked += int(invocationCount)
				log.Info("Iteration spent: ", time.Since(iterStart), "\tMinute Nbr. ", minute)
				log.Info("Required #invocations=", totalNumInvocationsEachMinute[minute],
					" Fired #functions=", numFuncInvocaked, "\tMinute Nbr. ", minute)

				invocRecord.Duration = time.Since(iterStart).Microseconds()
				invocRecord.IdleDuration = idleDuration.Microseconds()
				invocRecord.NumFuncRequested = totalNumInvocationsEachMinute[minute]
				invocRecord.NumFuncInvoked = numFuncInvocaked
				invocRecord.NumFuncFailed = numFuncToInvokeThisMinute - numFuncInvocaked
				invocRecords = append(invocRecords, &invocRecord)
				goto next_minute //* `break` doesn't work here as it's somehow ambiguous to Golang.
			}
			next++
		}
	next_minute:
	}
	log.Info("\tFinished invoking all functions.\n\tStart waiting for all requests to return.")

	//* Hyperparameter for busy-wait
	delta := time.Duration(1) //TODO: Make this force wait customisable (currently it's the same duration as that of the traces).
	forceTimeoutDuration := time.Duration(totalDurationMinutes) * time.Minute / delta

	if !withBlocking {
		forceTimeoutDuration = time.Second * 2
	}

	if wgWaitWithTimeout(&wg, forceTimeoutDuration) {
		log.Warn("Time out waiting for ALL invocations to return.")
	} else {
		totalDuration := time.Since(start)
		log.Info("[No time out] Total invocation + waiting duration: ", totalDuration, "\tIdle ", idleDuration, "\n")
	}

	//TODO: Extract IO out.
	invocFileName := "data/out/invoke_phase-" + strconv.Itoa(phaseIdx) + "_dur-" + strconv.Itoa(len(invocRecords)) + ".csv"
	invocF, err := os.Create(invocFileName)
	util.Check(err)
	gocsv.MarshalFile(&invocRecords, invocF)

	latencyFileName := "data/out/latency_phase-" + strconv.Itoa(phaseIdx) + "_dur-" + strconv.Itoa(len(invocRecords)) + ".csv"
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
