package invokefunc

import (
	"math"
	"sync"
	"time"

	"github.com/eth-easl/loader/pkg/common"
)

var (
	workloadDict      = make(map[string]interface{})
	totalRequestedGPU = 0
	rwMutex           = sync.RWMutex{}
)

var (
	traceInSched = make(map[string]int)
	rwTraceMutex = sync.RWMutex{}
)

type PTInfo struct {
	Deadline  time.Time
	Iteration int
	GPU       int
}

func setValue(key string, value interface{}) {
	rwMutex.Lock()
	defer rwMutex.Unlock()
	workloadDict[key] = value
	totalRequestedGPU += workloadDict[key].(int)
}

func getValue(key string) interface{} {
	rwMutex.RLock()
	defer rwMutex.RUnlock()
	return workloadDict[key]
}

func deleteValue(key string) {
	rwMutex.Lock()
	if _, ok := workloadDict[key].(int); ok {
		totalRequestedGPU -= workloadDict[key].(int)
	}
	delete(workloadDict, key)
	rwMutex.Unlock()
}

func queryFairGPUCount(key string) int {
	rwMutex.RLock()
	defer rwMutex.RUnlock()
	value, ok := workloadDict[key].(int)
	if !ok {
		return 0
	}
	return int(math.Ceil(float64(common.TotalGPUs*value) / float64(totalRequestedGPU)))
}

func setSchedJobCount(key string) {
	rwTraceMutex.Lock()
	traceInSched[key] = 1
	rwTraceMutex.Unlock()
}

func removeSchedJobCount(key string) {
	rwTraceMutex.Lock()
	delete(traceInSched, key)
	rwTraceMutex.Unlock()
}

func QueryJobInScheduleCount() int {
	rwTraceMutex.RLock()
	defer rwTraceMutex.RUnlock()
	return len(traceInSched)
}

// func calculateJobWeight() {
// 	go func() {
// 		for {
// 			rwMutex.RLock()
// 			// Perform the calculation for each key in the workloadDict map
// 			for key, value := range workloadDict {
// 				// Perform the calculation for job weight here
// 				// You can access the key and value in the loop
// 				// and calculate the weight accordingly
// 			}
// 			rwMutex.RUnlock()

// 			time.Sleep(5 * time.Second) // Sleep for 5 seconds before the next execution
// 		}
// 	}()
// }
