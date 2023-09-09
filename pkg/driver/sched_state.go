package driver

import (
	"sync"
)

var (
	finish  = false
	rwMutex = sync.RWMutex{}
)

func QueryFinish() bool {
	rwMutex.RLock()
	defer rwMutex.RUnlock()
	return finish
}


func SetFinish() {
	rwMutex.Lock()
	finish = true 
	rwMutex.Unlock()
}
