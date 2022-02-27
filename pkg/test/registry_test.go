package test

import (
	"strconv"
	"sync"
	"testing"

	fc "github.com/eth-easl/loader/pkg/function"
	"github.com/stretchr/testify/assert"
)

func TestConcurrentRegistration(t *testing.T) {
	registry := fc.LoadRegistry{}
	funcName := "func"
	memoryRequested := 1
	var wg sync.WaitGroup

	wg.Add(1000)
	for i := 0; i < 1000; i++ {
		go func() {
			registry.Register(funcName, memoryRequested)
			wg.Done()
		}()
	}
	wg.Wait()

	assert.Equal(t, 1000, int(registry.GetTotalMemoryLoad()))

	wg.Add(50)
	for i := 0; i < 50; i++ {
		go func() {
			registry.Deregister(funcName, memoryRequested)
			wg.Done()
		}()
	}
	wg.Wait()

	assert.Equal(t, 950, int(registry.GetTotalMemoryLoad()))
}

func TestRegisterDiffFunc(t *testing.T) {
	registry := fc.LoadRegistry{}
	funcName := "func"
	memoryRequested := 1

	for i := 0; i < 100; i++ {
		registry.Register(funcName+strconv.Itoa(i), memoryRequested)
	}
	assert.Equal(t, 100, int(registry.GetTotalMemoryLoad()))

	for i := 0; i < 50; i++ {
		registry.Deregister(funcName+strconv.Itoa(i), memoryRequested)
	}
	assert.Equal(t, 50, int(registry.GetTotalMemoryLoad()))

}

func TestRegisterSameFunc(t *testing.T) {
	registry := fc.LoadRegistry{}
	funcName := "func"
	memoryRequested := 1

	for i := 0; i < 100; i++ {
		registry.Register(funcName, memoryRequested)
	}
	assert.Equal(t, 100, int(registry.GetTotalMemoryLoad()))

	for i := 0; i < 50; i++ {
		registry.Deregister(funcName, memoryRequested)
	}
	assert.Equal(t, 50, int(registry.GetTotalMemoryLoad()))
}
