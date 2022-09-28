package driver

import (
	"github.com/eth-easl/loader/pkg/common"
	fc "github.com/eth-easl/loader/pkg/function"
	"github.com/eth-easl/loader/pkg/generator"
	log "github.com/sirupsen/logrus"
	"sync"
	"sync/atomic"
	"time"

	mc "github.com/eth-easl/loader/pkg/metric"
	tc "github.com/eth-easl/loader/pkg/trace"
)

type DriverConfiguration struct {
	EnableMetricsCollection bool
	IATDistribution         common.IatDistribution

	SampleSize                    int
	PhaseIdx                      int
	PhaseOffset                   int
	WithBlocking                  bool
	Functions                     []tc.Function
	InvocationsEachMinute         [][]int
	TotalNumInvocationsEachMinute []int
	WithTracing                   bool
	Seed                          int64
}

type Driver struct {
	collector            mc.Collector
	clusterUsage         mc.ClusterUsage
	knStats              mc.KnStats
	coldStartGauge       int
	coldStartMinuteCount int // TODO: maybe set to -1 if scraping is not enabled

	Configuration          *DriverConfiguration
	SpecificationGenerator *generator.SpecificationGenerator
}

func NewDriver(driverConfig *DriverConfiguration) *Driver {
	return &Driver{
		Configuration:          driverConfig,
		SpecificationGenerator: generator.NewSpecificationGenerator(driverConfig.Seed),
	}
}

/////////////////////////////////////////
// METRICS SCRAPPERS
/////////////////////////////////////////

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

/////////////////////////////////////////
// DRIVER LOGIC
/////////////////////////////////////////

func (d *Driver) invokeFunction(function tc.Function, announceInvocationDone *sync.WaitGroup,
	runtimeSpecs *generator.RuntimeSpecification, successCount *int64, failedCount *int64) {

	defer announceInvocationDone.Done()

	success, _ := fc.Invoke(function, runtimeSpecs, d.Configuration.WithTracing)

	if success {
		atomic.AddInt64(successCount, 1)
	} else {
		atomic.AddInt64(failedCount, 1)
	}

	/*execRecord.Phase = phase
	execRecord.Interval = interval
	execRecord.Rps = rps
	if d.Configuration.EnableMetricsCollection {
		execRecord.ColdStartCount = d.coldStartGauge
		d.collector.ReportExecution(execRecord, d.clusterUsage, d.knStats)
	}*/
}

func (d *Driver) individualFunctionDriver(function tc.Function, announceFunctionDone *sync.WaitGroup,
	totalSuccessfull *int64, totalFailed *int64) {

	totalTraceDuration := len(d.Configuration.TotalNumInvocationsEachMinute)
	minuteIndex, invocationIndex := 0, 0

	spec := d.SpecificationGenerator.GenerateInvocationData(function, d.Configuration.IATDistribution)
	IAT, runtimeSpecification := spec.IAT, spec.RuntimeSpecification

	var successfullInvocations int64
	var failedInvocations int64
	waitForInvocations := sync.WaitGroup{}

	for {
		// Check whether the end of trace has been reached
		if minuteIndex >= totalTraceDuration {
			break
		}

		if function.NumInvocationsPerMinute[minuteIndex] == 0 {
			minuteIndex++
			invocationIndex = 0

			time.Sleep(time.Minute)
		} else {
			waitForInvocations.Add(1)

			go d.invokeFunction(function,
				&waitForInvocations,
				&runtimeSpecification[minuteIndex][invocationIndex],
				&successfullInvocations,
				&failedInvocations)

			sleepFor := time.Duration(IAT[minuteIndex][invocationIndex]) * time.Microsecond

			invocationIndex++
			if function.NumInvocationsPerMinute[minuteIndex] == invocationIndex {
				minuteIndex++
				invocationIndex = 0

				if minuteIndex >= totalTraceDuration {
					break
				}
			}

			//fmt.Println(sleepFor)
			time.Sleep(sleepFor)
		}
	}

	waitForInvocations.Wait()
	announceFunctionDone.Done()

	atomic.AddInt64(totalSuccessfull, successfullInvocations)
	atomic.AddInt64(totalFailed, failedInvocations)
}

func (d *Driver) globalTimekeeper(totalTraceDuration int) {
	ticker := time.NewTicker(time.Minute)
	globalTimeCounter := 0

	for {
		<-ticker.C

		if globalTimeCounter != 0 {
			log.Infof("End of minute %d\n", globalTimeCounter)
		}

		globalTimeCounter++
		if globalTimeCounter >= totalTraceDuration {
			break
		}

		log.Infof("Start of minute %d\n", globalTimeCounter)
	}

	ticker.Stop()
}

func (d *Driver) GenerateTraceLoads() int {
	// TODO: need assert on trace parsing that the last column with non null is parsed and declared as trace length
	totalTraceDuration := len(d.Configuration.TotalNumInvocationsEachMinute)

	if d.Configuration.EnableMetricsCollection {
		// TODO: these following arguments should be configurable
		go d.CreateKnativeMetricsScrapper(time.Second * 15)
		go d.CreateKnativeStateUpdateScrapper(time.Second * 15)
		go d.CreateColdStartCountScrapper(time.Second * 60)
	}

	go d.globalTimekeeper(totalTraceDuration)

	var successfullInvocations int64
	var failedInvocations int64
	allFunctionsCompleted := sync.WaitGroup{}

	for i, function := range d.Configuration.Functions {
		if i > 0 {
			break // FOR DEBUGGING CURRENTLY
		}

		allFunctionsCompleted.Add(1)
		go d.individualFunctionDriver(function, &allFunctionsCompleted, &successfullInvocations, &failedInvocations)
	}

	allFunctionsCompleted.Wait()

	log.Infof("Number of successful invocations: %d\n", successfullInvocations)
	log.Infof("Number of failed invocations: %d\n", failedInvocations)

	return 0
}

/////////////////////////////////////
// TODO: check and refactor everything below
/////////////////////////////////////

/**
 * This function waits for the waitgroup for the specified max timeout.
 * Returns true if waiting timed out.
 */
/*func wgWaitWithTimeout(wg *sync.WaitGroup, timeout time.Duration) bool {
	log.Info("Start waiting for all requests to return.")
	c := make(chan struct{})
	go func() {
		defer close(c)
		wg.Wait()
	}()
	select {
	case <-c:
		log.Info("Finished waiting for all invocations.")
		return false
	case <-time.After(timeout):
		return true
	}
}*/

/*func CheckOverload(successCount, failureCount int64) bool {
	//* Amongst those returned, how many has failed?
	failureRate := float64(failureCount) / float64(successCount+failureCount)
	log.Info("Failure rate=", failureRate)
	return failureRate > common.OVERFLOAD_THRESHOLD
}

func DumpOverloadFlag() {
	// If the file doesn't exist, create it, or append to the file
	_, err := os.OpenFile("overload.flag", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatal(err)
	}
}*/
