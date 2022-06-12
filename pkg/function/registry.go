package function

import (
	"sync"
	"sync/atomic"

	util "github.com/eth-easl/loader/pkg"
)

var registry = LoadRegistry{}

type LoadRegistry struct {
	mutex     sync.Mutex
	loadGauge int64
}

func (r *LoadRegistry) Register(memoryRequested int) {
	atomic.AddInt64(&r.loadGauge, int64(memoryRequested))
}

func (r *LoadRegistry) Deregister(memoryRequested int) {
	r.mutex.Lock()

	atomic.AddInt64(&r.loadGauge, -1*int64(memoryRequested))
	atomic.StoreInt64(&r.loadGauge, int64(util.MaxOf(0, int(r.loadGauge))))

	r.mutex.Unlock()
}

func (r *LoadRegistry) GetTotalMemoryLoad() int64 {
	return atomic.LoadInt64(&r.loadGauge)
}
