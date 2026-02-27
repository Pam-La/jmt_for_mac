package async

import (
	"errors"
	"runtime"
	"sync/atomic"
)

var (
	ErrInvalidCapacity = errors.New("ring buffer capacity must be a power of two and >= 2")
)

type slot[T any] struct {
	sequence atomic.Uint64
	value    T
}

// RingBuffer는 lock-free MPMC bounded queue이다.
// 알고리즘은 sequence 기반 CAS 패턴(Vyukov 스타일)을 따른다.
type RingBuffer[T any] struct {
	capacity uint64
	mask     uint64

	_pad0 [48]byte
	head  atomic.Uint64
	_pad1 [48]byte
	tail  atomic.Uint64
	_pad2 [48]byte

	slots []slot[T]
}

func NewRingBuffer[T any](capacity uint64) (*RingBuffer[T], error) {
	if capacity < 2 || (capacity&(capacity-1)) != 0 {
		return nil, ErrInvalidCapacity
	}
	slots := make([]slot[T], capacity)
	for i := uint64(0); i < capacity; i++ {
		slots[i].sequence.Store(i)
	}
	return &RingBuffer[T]{
		capacity: capacity,
		mask:     capacity - 1,
		slots:    slots,
	}, nil
}

func (q *RingBuffer[T]) Enqueue(value T) bool {
	for {
		pos := q.tail.Load()
		slot := &q.slots[pos&q.mask]
		seq := slot.sequence.Load()
		delta := int64(seq) - int64(pos)

		if delta == 0 {
			if q.tail.CompareAndSwap(pos, pos+1) {
				slot.value = value
				slot.sequence.Store(pos + 1)
				return true
			}
			continue
		}
		if delta < 0 {
			return false
		}
		runtime.Gosched()
	}
}

func (q *RingBuffer[T]) Dequeue() (T, bool) {
	var zero T
	for {
		pos := q.head.Load()
		slot := &q.slots[pos&q.mask]
		seq := slot.sequence.Load()
		delta := int64(seq) - int64(pos+1)

		if delta == 0 {
			if q.head.CompareAndSwap(pos, pos+1) {
				value := slot.value
				slot.value = zero
				slot.sequence.Store(pos + q.capacity)
				return value, true
			}
			continue
		}
		if delta < 0 {
			return zero, false
		}
		runtime.Gosched()
	}
}
