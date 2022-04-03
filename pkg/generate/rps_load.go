package generate

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	log "github.com/sirupsen/logrus"

	fc "github.com/eth-easl/loader/pkg/function"
	mc "github.com/eth-easl/loader/pkg/metric"
	tc "github.com/eth-easl/loader/pkg/trace"
)

func GenerateStressLoads(
	rpsStart int,
	rpsStep int,
	stressSlotInSecs int,
	functions []tc.Function,
) {
	start := time.Now()
	wg := sync.WaitGroup{}
	collector := mc.NewCollector()
	clusterUsage := mc.ClusterUsage{}
	knStats := mc.KnStats{}
	coldStartSlotCount := 0

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
			coldStartSlotCount += collector.GetColdStartCount()
		}
	}()

	rps := rpsStart
	tolerance := 0

stress_generation:
	for {
		iats := GenerateInterarrivalTimesInMicro(
			rps*60,
			true,
		)
		tick := -1
		timeout := time.After(time.Second * time.Duration(stressSlotInSecs))
		interval := time.Duration(iats[0]) * time.Microsecond
		ticker := time.NewTicker(interval)
		done := make(chan bool, 1)

		//* The following counters are for each RPS step slot.
		var successCountRpsStep int64 = 0
		var failureCountRpsStep int64 = 0

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
				//* Invoke functions using round robin.
				function := functions[tick%len(functions)]

				go func(rps int, interval int64) {
					defer wg.Done()
					wg.Add(1)

					//* Use the maximum socket timeout from AWS (1min).
					diallingTimeout := 1 * time.Minute
					ctx, cancel := context.WithTimeout(context.Background(), diallingTimeout)
					defer cancel()

					success, execRecord := fc.Invoke(ctx, function, GenerateSingleExecutionSpecs)

					if success {
						atomic.AddInt64(&successCountRpsStep, 1)
					} else {
						atomic.AddInt64(&failureCountRpsStep, 1)
					}
					execRecord.Interval = interval
					execRecord.Rps = rps
					collector.ReportExecution(execRecord, clusterUsage, knStats)
				}(rps, interval.Milliseconds()) //* NB: `clusterUsage` needn't be pushed onto the stack as we want the latest.

			case <-done:
				invRecord := mc.MinuteInvocationRecord{
					Rps:           rps,
					NumColdStarts: coldStartSlotCount,
				}
				collector.ReportInvocation(invRecord)
				coldStartSlotCount = 0

				if CheckOverload(atomic.LoadInt64(&successCountRpsStep), atomic.LoadInt64(&failureCountRpsStep)) {
					tolerance++
					if tolerance < OVERFLOAD_TOLERANCE {
						rps -= rpsStep //* Stay in the current RPS for one more time.
						goto next_rps
					} else {
						break stress_generation
					}
				} else {
					goto next_rps
				}
			}
		}
	next_rps:
		if rpsStep == 0 { // For a single shot.
			break stress_generation
		}
		rps += rpsStep
		log.Info("Start next round with RPS=", rps, " after ", time.Since(start))
	}
	log.Info("Finished stress load generation with ending RPS=", rps)

	forceTimeoutDuration := 15 * time.Minute
	if wgWaitWithTimeout(&wg, forceTimeoutDuration) {
		log.Warn("Time out waiting for all invocations to return.")
	} else {
		totalDuration := time.Since(start)
		log.Info("[No time out] Total invocation + waiting duration: ", totalDuration, "\n")
	}

	defer collector.FinishAndSave(9999, 0, rps*stressSlotInSecs)
}
