package common

import (
	"sync"
	"sync/atomic"
	"testing"
)

func TestAtomicQueueNonParallel(t *testing.T) {
	queue := NewLockFreeQueue[int]()

	queue.Enqueue(1)
	queue.Enqueue(2)
	queue.Enqueue(3)
	queue.Enqueue(4)
	queue.Enqueue(5)

	if queue.Length() != 5 {
		t.Errorf("queue length should be 5")
	}

	sum := 0
	for queue.Length() > 0 {
		val := queue.Dequeue()
		sum += val
	}

	if queue.Length() != 0 {
		t.Errorf("queue length should be 0")
	}
	if sum != 15 {
		t.Errorf("sum should be equal 15")
	}
}

func TestAtomicQueueParallel(t *testing.T) {
	queue := NewLockFreeQueue[int]()
	//queue := NewQueue()

	values := 1_000_000

	wg := sync.WaitGroup{}
	wg.Add(values)
	for i := 0; i < values; i++ {
		go func(idx int) {
			defer wg.Done()

			queue.Enqueue(idx)
		}(i)
	}
	wg.Wait()

	if queue.Length() != values {
		t.Errorf("queue length should be %d", values)
	}

	sum := int64(0)

	wg.Add(values)
	for i := 0; i < values; i++ {
		go func() {
			defer wg.Done()

			val := queue.Dequeue()
			atomic.AddInt64(&sum, int64(val))
		}()
	}
	wg.Wait()

	if queue.Length() != 0 {
		t.Errorf("queue length should be 0")
	}
	if int(sum) != values*(values-1)/2 {
		t.Errorf("sum should be equal %d, but got %d", values*(values-1)/2, sum)
	}
}
