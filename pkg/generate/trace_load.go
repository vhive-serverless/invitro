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

type TraceGeneratorParams struct {
	EnableMetricsCollection bool

	SampleSize                    int
	PhaseIdx                      int
	PhaseOffset                   int
	WithBlocking                  bool
	Functions                     []tc.Function
	InvocationsEachMinute         [][]int
	TotalNumInvocationsEachMinute []int
	IatDistribution               IatDistribution
	WithTracing                   bool
	Seed                          int64
}

type Driver struct {
	collector            mc.Collector
	clusterUsage         mc.ClusterUsage
	knStats              mc.KnStats
	coldStartGauge       int
	coldStartMinuteCount int // TODO: maybe set to -1 if scraping is not enabled
}

func NewDriver() *Driver {
	return &Driver{}
}

// CreateKnativeMetricsScrapper launches a scraper that updates the cluster usage periodically
func (d *Driver) CreateKnativeMetricsScrapper(interval time.Duration) func() {
	timer := time.NewTicker(interval)
	d.collector = mc.NewCollector()

	return func() {
		for {
			<-timer.C
			d.clusterUsage = mc.ScrapeClusterUsage()
		}
	}
}

// CreateColdStartCountScrapper creates cold start count scrapper with the given period
func (d *Driver) CreateColdStartCountScrapper(interval time.Duration) func() {
	timer := time.NewTicker(time.Second * 60)
	d.knStats = mc.KnStats{}
	d.coldStartGauge = 0
	d.coldStartMinuteCount = 0

	return func() {
		for {
			<-timer.C
			d.coldStartGauge = d.collector.RecordScalesAndGetColdStartCount()
			d.coldStartMinuteCount += d.coldStartGauge
		}
	}
}

// CreateKnativeStateUpdateScrapper creates a scraper that updates Knative states periodically
func (d *Driver) CreateKnativeStateUpdateScrapper(interval time.Duration) func() {
	timer := time.NewTicker(interval)
	d.clusterUsage = mc.ClusterUsage{}

	return func() {
		for {
			<-timer.C
			d.knStats = mc.ScrapeKnStats()
		}
	}
}

func (d *Driver) GenerateTraceLoads(params TraceGeneratorParams) int {
	sg := NewSpecificationGenerator(params.Seed)
	// TODO: need assert on trace parsing that the last column with non null is parsed and declared as trace length
	totalTraceDuration := len(params.TotalNumInvocationsEachMinute)

	if params.EnableMetricsCollection {
		// TODO: these following arguments should be configurable
		go d.CreateKnativeMetricsScrapper(time.Second * 15)
		go d.CreateKnativeStateUpdateScrapper(time.Second * 15)
		go d.CreateColdStartCountScrapper(time.Second * 60)
	}

	start := time.Now()
	wg := sync.WaitGroup{}

	//* The following counters are for the whole measurement (we don't stop in the middle).
	var successCountTotal int64 = 0
	var failureCountTotal int64 = 0

	var minute int
trace_gen:
	for minute = 0; minute < totalTraceDuration; minute++ {
		var iats [][]float64
		var numFuncInvokedThisMinute int64 = 0

		rps := int(math.Ceil(float64(params.TotalNumInvocationsEachMinute[minute]) / 60.0))

		//* Bound the #invocations/minute by RPS.
		numInvocationsThisMinute := params.TotalNumInvocationsEachMinute[minute]
		if numInvocationsThisMinute < 1 {
			continue
		}

		iats, _ = sg.GenerateIAT([]int{numInvocationsThisMinute}, params.IatDistribution)
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
					funcIndx := params.InvocationsEachMinute[m][nxt]
					function := params.Functions[funcIndx]

					runtimeRequested, memoryRequested := sg.GenerateExecutionSpecs(function)
					success, execRecord := fc.Invoke(function, runtimeRequested, memoryRequested, params.WithTracing)

					if success {
						atomic.AddInt64(&successCountTotal, 1)
					} else {
						atomic.AddInt64(&failureCountTotal, 1)
					}
					execRecord.Phase = phase
					execRecord.Interval = interval
					execRecord.Rps = rps
					if params.EnableMetricsCollection {
						execRecord.ColdStartCount = d.coldStartGauge
						d.collector.ReportExecution(execRecord, d.clusterUsage, d.knStats)
					}

				}(minute, tick, params.PhaseIdx, rps, interval.Milliseconds()) //* Push vars onto the stack to prevent racing.

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
				log.Info("Target #invocations=", params.TotalNumInvocationsEachMinute[minute], " Fired #functions=", numFuncInvokedThisMinute, "\tMinute Nbr. ", minute)
				//! No reason to note down the failure rate here since many requests would still be on their way.
				invRecord := mc.MinuteInvocationRecord{
					MinuteIdx:       minute + params.PhaseOffset,
					Phase:           params.PhaseIdx,
					Rps:             rps,
					Duration:        time.Since(iterStart).Microseconds(),
					NumFuncTargeted: params.TotalNumInvocationsEachMinute[minute],
					NumFuncInvoked:  int(numFuncInvokedThisMinute),
				}

				if params.EnableMetricsCollection {
					invRecord.NumColdStarts = d.coldStartMinuteCount
					d.collector.ReportInvocation(invRecord)
					d.coldStartMinuteCount = 0
				}

				/** Warmup phases */
				stationaryWindow := 1
				if params.PhaseIdx == 1 && minute+1 >= WARMUP_DURATION_MINUTES {
					if params.EnableMetricsCollection {
						// TODO: should the coller always be running?
						if !d.collector.IsLatencyStationary(rps*60*stationaryWindow, STATIONARY_P_VALUE) {
							log.Warnf("Warmup may need longer than %d minutes", WARMUP_DURATION_MINUTES)
						}
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
	if !params.WithBlocking {
		forceTimeoutDuration = time.Second * 1
	}

	if wgWaitWithTimeout(&wg, forceTimeoutDuration) {
		log.Warn("Time out waiting for all invocations to return.")
	} else {
		totalDuration := time.Since(start)
		log.Info("[No timeout] Total invocation + waiting duration: ", totalDuration, "\n")
	}

	//* Only check overload after the entire Phase 2 to account for all late returns.
	if params.PhaseIdx == 2 && CheckOverload(atomic.LoadInt64(&successCountTotal), atomic.LoadInt64(&failureCountTotal)) {
		DumpOverloadFlag()
	}

	if params.EnableMetricsCollection {
		// TODO: do we need defer here? everything above should be blocking and sequential
		defer d.collector.FinishAndSave(params.SampleSize, params.PhaseIdx, minute)
	}

	return params.PhaseOffset + minute
}
