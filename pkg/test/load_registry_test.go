package test

import (
	"sync"
	"testing"

	fc "github.com/eth-easl/loader/pkg/function"
	"github.com/stretchr/testify/assert"
)

func TestConcurrentRegistration(t *testing.T) {
	registry := fc.LoadRegistry{}
	memoryRequested := 1
	var wg sync.WaitGroup

	wg.Add(1000)
	for i := 0; i < 1000; i++ {
		go func() {
			registry.Register(memoryRequested)
			wg.Done()
		}()
	}
	wg.Wait()

	assert.Equal(t, 1000, int(registry.GetTotalMemoryLoad()))

	wg.Add(50)
	for i := 0; i < 50; i++ {
		go func() {
			registry.Deregister(memoryRequested)
			wg.Done()
		}()
	}
	wg.Wait()

	assert.Equal(t, 950, int(registry.GetTotalMemoryLoad()))

	wg.Add(50_000)
	for i := 0; i < 50_000; i++ {
		go func() {
			registry.Deregister(memoryRequested)
			wg.Done()
		}()
	}
	wg.Wait()

	assert.Equal(t, 0, int(registry.GetTotalMemoryLoad()))
}
