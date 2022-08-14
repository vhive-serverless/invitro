package generate

import (
	"sync"
	"sync/atomic"
	"time"

	log "github.com/sirupsen/logrus"

	fc "github.com/eth-easl/loader/pkg/function"
	mc "github.com/eth-easl/loader/pkg/metric"
	tc "github.com/eth-easl/loader/pkg/trace"
)

func GenerateBurstLoads(
	rpsTarget int,
	burstTarget int,
	burstDurationMinutes int,
	functionsTable map[string]tc.Function,
	iatDistribution IatDistribution,
	withTracing bool,
) {

	start := time.Now()
	wg := sync.WaitGroup{}
	collector := mc.NewCollector()
	clusterUsage := mc.ClusterUsage{}
	knStats := mc.KnStats{}
	coldStartMinuteCount := 0
	roundrobinFunctions := []string{"steady", "bursty"}

	/** Launch a scraper that updates the cluster usage every 15s (max. interval). */
	scrape_infra := time.NewTicker(time.Second * 15)
	go func() {
		for {
			<-scrape_infra.C
			clusterUsage = mc.ScrapeClusterUsage()
		}
	}()

	/** Launch a scraper that updates Knative states every 2s (max. interval). */
	scrape_kn := time.NewTicker(time.Second * 2)
	go func() {
		for {
			<-scrape_kn.C
			knStats = mc.ScrapeKnStats()
		}
	}()

	/** Launch a scraper for getting cold-start count. */
	scrape_scales := time.NewTicker(time.Second * 1)
	go func() {
		for {
			<-scrape_scales.C
			coldStartMinuteCount += collector.RecordScalesAndGetColdStartCount()
		}
	}()

	rps := 1
	minute := 0
	burstSize := 0
	durationMinutes := 1 // Before the burst period, each RPS step takes a minute.

burst_gen:
	for {

		if rps == rpsTarget {
			durationMinutes = burstDurationMinutes
		}

		iats := GenerateOneMinuteInterarrivalTimesInMicro(
			rps*60,
			iatDistribution,
		)
		tick := -1
		timeout := time.After(time.Duration(durationMinutes) * time.Minute)
		interval := time.Duration(iats[0]) * time.Microsecond
		ticker := time.NewTicker(interval)
		done := make(chan bool, 1)

		//* The following counters are for each RPS step slot.
		var invocationCount int64 = 0
		var failureCount int64 = 0

		/** Launch a timer. */
		go func() {
			<-timeout
			ticker.Stop()
			done <- true
		}()

		for {
			tick++
			select {
			case <-ticker.C:
				function := functionsTable[roundrobinFunctions[tick%2]]

				nap := 0
				if rps == rpsTarget && tick == rps*60*durationMinutes/2 {
					log.Info("Burst starts!")
					/** Creating the burst in the middle of the `burstDurationMinutes`. */
					burstSize = burstTarget
					function = functionsTable["bursty"]
					nap = 1000 / (burstTarget + 1)
				}

				for i := 0; i < burstSize+1; i++ {
					if burstSize == burstTarget && i == burstTarget/2 {
						/** Invoking the victim function in the middle. */
						function = functionsTable["victim"]
					}
					go func(_function tc.Function, rps int, interval int64) {
						defer wg.Done()
						wg.Add(1)

						atomic.AddInt64(&invocationCount, 1)

						runtimeRequested, memoryRequested := GenerateExecutionSpecs(function)
						success, execRecord := fc.Invoke(_function, runtimeRequested, memoryRequested, withTracing)

						if !success {
							atomic.AddInt64(&failureCount, 1)
						}

						execRecord.Interval = interval
						execRecord.Rps = rps
						collector.ReportExecution(execRecord, clusterUsage, knStats)
					}(function, rps, interval.Milliseconds())
					//! Push function onto stack!!! (concurrency again problem again gosh...)

					if function.Name == "victim-func" {
						function = functionsTable["bursty"]
					}
					time.Sleep(time.Duration(nap) * time.Millisecond)
				}

				burstSize = 0
				nap = 0

			case <-done:
				invRecord := mc.MinuteInvocationRecord{
					MinuteIdx:       minute,
					Rps:             rps,
					NumFuncTargeted: int(invocationCount),
					//! This failure count is not representative as there could be requests still on their way (non-blocking).
					NumFuncInvoked: int(invocationCount) - int(failureCount),
					NumColdStarts:  coldStartMinuteCount,
				}
				collector.ReportInvocation(invRecord)
				coldStartMinuteCount = 0
				goto next_rps
			}
		}
	next_rps:
		if rps == rpsTarget {
			break burst_gen
		}
		minute += durationMinutes
		rps++
		log.Info("Start next round with RPS=", rps, " after ", time.Since(start))
	}
	log.Info("Finished burst generation with ending RPS=", rps)

	forceTimeoutDuration := FORCE_TIMEOUT_MINUTE * time.Minute
	if wgWaitWithTimeout(&wg, forceTimeoutDuration) {
		log.Warn("Time out waiting for all invocations to return.")
	} else {
		totalDuration := time.Since(start)
		log.Info("[No timeout] Total invocation + waiting duration: ", totalDuration, "\n")
	}

	defer collector.FinishAndSave(999, 999, rpsTarget+burstDurationMinutes)
}
