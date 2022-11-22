package generate

import (
	"math"
	"sync"
	"sync/atomic"
	"time"

	log "github.com/sirupsen/logrus"

	fc "github.com/eth-easl/loader/pkg/function"
	mc "github.com/eth-easl/loader/pkg/metric"
	tc "github.com/eth-easl/loader/pkg/trace"
)

func GenerateTraceLoads(sampleSize int, phaseIdx int, phaseOffset int, withBlocking bool, functions []tc.Function,
	invocationsEachMinute [][]int, totalNumInvocationsEachMinute []int, iatDistribution IatDistribution, withTracing bool,
	seed int64) int {

	collector := mc.NewCollector()
	clusterUsage := mc.ClusterUsage{}
	knStats := mc.KnStats{}
	coldStartGauge := 0
	coldStartMinuteCount := 0

	/** Launch a scraper that updates the cluster usage every 15s (max. interval). */
	scrape_infra := time.NewTicker(time.Second * 15)
	go func() {
		for {
			<-scrape_infra.C
			clusterUsage = mc.ScrapeClusterUsage()
		}
	}()

	/** Launch a scraper that updates Knative states every 15s (max. interval). */
	scrape_kn := time.NewTicker(time.Second * 15)
	go func() {
		for {
			<-scrape_kn.C
			knStats = mc.ScrapeKnStats()
		}
	}()

	/** Launch a scraper for getting cold-start count. */
	scrape_scales := time.NewTicker(time.Second * 60)
	go func() {
		for {
			<-scrape_scales.C
			coldStartGauge = collector.RecordScalesAndGetColdStartCount()
			coldStartMinuteCount += coldStartGauge
		}
	}()

	start := time.Now()
	wg := sync.WaitGroup{}
	totalDurationMinutes := len(totalNumInvocationsEachMinute)

	minute := 0
	//* The following counters are for the whole measurement (we don't stop in the middle).
	var successCountTotal int64 = 0
	var failureCountTotal int64 = 0

	sg := NewSpecificationGenerator(seed)

trace_gen:
	for ; minute < int(totalDurationMinutes); minute++ {
		var iats [][]float64
		var numFuncInvokedThisMinute int64 = 0

		rps := int(math.Ceil(float64(totalNumInvocationsEachMinute[minute]) / 60.0))

		//* Bound the #invocations/minute by RPS.
		numInvocationsThisMinute := totalNumInvocationsEachMinute[minute]
		if numInvocationsThisMinute < 1 {
			continue
		}

		iats, _ = sg.GenerateIAT([]int{numInvocationsThisMinute}, iatDistribution)
		log.Infof("Minute[%d]\t RPS=%d", minute, rps)

		/** Set up timer to bound the one-minute invocation. */
		iterStart := time.Now()
		timeout := time.After(time.Duration(60) * time.Second)
		interval := time.Duration(iats[0][0]) * time.Microsecond
		ticker := time.NewTicker(interval)
		done := make(chan bool, 2) // Two semaphores, one for timer, one for early completion.
		tick := 0

		wg.Add(1)
		/** Launch a timer. */
		go func() {
			defer wg.Done()

			t := <-timeout
			log.Warn("(Slot finished)\t", t.Format(time.StampMilli), "\tMinute Nbr. ", minute)
			ticker.Stop()
			done <- true
		}()

	this_minute_gen:
		for {
			select {
			case <-ticker.C:

				wg.Add(1)
				go func(m int, nxt int, phase int, rps int, interval int64) {
					defer wg.Done()

					atomic.AddInt64(&numFuncInvokedThisMinute, 1)
					funcIndx := invocationsEachMinute[m][nxt]
					function := functions[funcIndx]

					runtimeRequested, memoryRequested := sg.GenerateExecutionSpecs(function)
					success, execRecord := fc.Invoke(function, runtimeRequested, memoryRequested, withTracing)

					if success {
						atomic.AddInt64(&successCountTotal, 1)
					} else {
						atomic.AddInt64(&failureCountTotal, 1)
					}
					execRecord.Phase = phase
					execRecord.Interval = interval
					execRecord.Rps = rps
					execRecord.ColdStartCount = coldStartGauge
					collector.ReportExecution(execRecord, clusterUsage, knStats)

				}(minute, tick, phaseIdx, rps, interval.Milliseconds()) //* Push vars onto the stack to prevent racing.

				tick++
				if tick >= numInvocationsThisMinute {
					//* Finished before timeout.
					log.Info("Finish target invocation early at Minute slot ", minute, " Itr. ", tick)
					done <- true
				} else {
					//* Load the next inter-arrival time.
					interval = time.Duration(iats[0][tick]) * time.Microsecond
					ticker = time.NewTicker(interval)
				}

			case <-done:
				log.Info("Iteration spent: ", time.Since(iterStart), "\tMinute Nbr. ", minute)
				log.Info("Target #invocations=", totalNumInvocationsEachMinute[minute], " Fired #functions=", numFuncInvokedThisMinute, "\tMinute Nbr. ", minute)
				//! No reason to note down the failure rate here since many requests would still be on their way.
				invRecord := mc.MinuteInvocationRecord{
					MinuteIdx:       minute + phaseOffset,
					Phase:           phaseIdx,
					Rps:             rps,
					Duration:        time.Since(iterStart).Microseconds(),
					NumFuncTargeted: totalNumInvocationsEachMinute[minute],
					NumFuncInvoked:  int(numFuncInvokedThisMinute),
					NumColdStarts:   coldStartMinuteCount,
				}
				//* Export metrics for all phases.
				collector.ReportInvocation(invRecord)
				coldStartMinuteCount = 0

				/** Warmup phases */
				stationaryWindow := 1
				if phaseIdx == 1 && minute+1 >= WARMUP_DURATION_MINUTES {
					if !collector.IsLatencyStationary(rps*60*stationaryWindow, STATIONARY_P_VALUE) {
						log.Warnf("Warmup may need longer than %d minutes", WARMUP_DURATION_MINUTES)
					}
					minute++
					break trace_gen
				}

				break this_minute_gen
			}
		}
	}
	log.Info("\tFinished invoking all functions.")

	forceTimeoutDuration := time.Duration(FORCE_TIMEOUT_MINUTE) * time.Minute
	if !withBlocking {
		forceTimeoutDuration = time.Second * 1
	}

	if wgWaitWithTimeout(&wg, forceTimeoutDuration) {
		log.Warn("Time out waiting for all invocations to return.")
	} else {
		totalDuration := time.Since(start)
		log.Info("[No timeout] Total invocation + waiting duration: ", totalDuration, "\n")
	}

	//* Only check overload after the entire Phase 2 to account for all late returns.
	if phaseIdx == 2 && CheckOverload(atomic.LoadInt64(&successCountTotal), atomic.LoadInt64(&failureCountTotal)) {
		DumpOverloadFlag()
	}

	defer collector.FinishAndSave(sampleSize, phaseIdx, minute)

	return phaseOffset + minute
}
