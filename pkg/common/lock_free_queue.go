package common

import (
	"github.com/sirupsen/logrus"
	"sync/atomic"
	"unsafe"
)

// LockFreeQueue reference implementation from The Art of Multiprocessor Programming pg. 236
type LockFreeQueue[T any] struct {
	head unsafe.Pointer
	tail unsafe.Pointer

	length int32
}

type lfqElement[T any] struct {
	value T
	next  unsafe.Pointer
}

func NewLockFreeQueue[T any]() *LockFreeQueue[T] {
	queue := &LockFreeQueue[T]{}
	item := &lfqElement[T]{}

	queue.head = unsafe.Pointer(item)
	queue.tail = unsafe.Pointer(item)

	return queue
}

func (lfq *LockFreeQueue[T]) Enqueue(item T) {
	defer atomic.AddInt32(&lfq.length, 1)

	node := &lfqElement[T]{
		value: item,
		next:  nil,
	}

	for {
		last := (*lfqElement[T])(lfq.tail)
		next := last.next

		if last == (*lfqElement[T])(lfq.tail) {
			if next == nil {
				if atomic.CompareAndSwapPointer(&last.next, next, unsafe.Pointer(node)) {
					atomic.CompareAndSwapPointer(&lfq.tail, unsafe.Pointer(last), unsafe.Pointer(node))
					break
				}
			} else {
				atomic.CompareAndSwapPointer(&lfq.tail, unsafe.Pointer(last), next)
			}
		}
	}

}

func (lfq *LockFreeQueue[T]) Dequeue() T {
	defer atomic.AddInt32(&lfq.length, -1)

	for {
		first := (*lfqElement[T])(lfq.head)
		last := (*lfqElement[T])(lfq.tail)
		next := first.next

		if first == (*lfqElement[T])(lfq.head) {
			if first == last {
				if next == nil {
					logrus.Fatal("No element to dequeue.")
				}
			} else {
				value := (*lfqElement[T])(next).value
				if atomic.CompareAndSwapPointer(&lfq.head, unsafe.Pointer(first), next) {
					return value
				}
			}
		}
	}
}

func (lfq *LockFreeQueue[T]) Length() int {
	return int(atomic.LoadInt32(&lfq.length))
}
