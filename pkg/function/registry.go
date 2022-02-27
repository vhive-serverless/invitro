package function

import (
	"sync"

	log "github.com/sirupsen/logrus"
)

type LoadRegistry struct {
	rwMutex  sync.RWMutex
	registry sync.Map
}

/**
 * * We use function names for registration as they are created uniquely
 * *  and also more readable for debugging.
 */
func (r *LoadRegistry) Register(name string, memoryRequested int) {
	r.rwMutex.Lock()

	registeredMem, exist := r.registry.LoadOrStore(name, memoryRequested)
	if exist {
		//* If an instance of this function exists, register the load under the same name.
		r.registry.Store(name, registeredMem.(int)+memoryRequested)
	}

	r.rwMutex.Unlock()
}

func (r *LoadRegistry) Deregister(name string, memoryRequested int) {
	r.rwMutex.Lock()

	registeredMem, exist := r.registry.LoadAndDelete(name)
	if !exist {
		log.Fatal("Error in deregistering : ", name, " (NOT exist)")
	}

	remainingLoad := registeredMem.(int) - memoryRequested
	if remainingLoad > 0 {
		//* If the registered load is of many instances, deregister the finished part and restore the rest back.
		r.registry.Store(name, remainingLoad)
	}

	r.rwMutex.Unlock()
}

func (r *LoadRegistry) GetTotalMemoryLoad() uint32 {
	r.rwMutex.RLock()

	var totalLoad uint32
	r.registry.Range(func(name, mem interface{}) bool {
		totalLoad += uint32(mem.(int))
		return true
	})

	r.rwMutex.RUnlock()
	return totalLoad
}
