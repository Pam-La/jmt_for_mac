package async

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRingBufferBasic(t *testing.T) {
	q, err := NewRingBuffer[int](8)
	if err != nil {
		t.Fatalf("new ring buffer failed: %v", err)
	}

	for i := 0; i < 8; i++ {
		if ok := q.Enqueue(i); !ok {
			t.Fatalf("enqueue failed at %d", i)
		}
	}
	if ok := q.Enqueue(99); ok {
		t.Fatalf("enqueue should fail when queue is full")
	}

	for i := 0; i < 8; i++ {
		got, ok := q.Dequeue()
		if !ok {
			t.Fatalf("dequeue failed at %d", i)
		}
		if got != i {
			t.Fatalf("unexpected dequeue value: got=%d want=%d", got, i)
		}
	}
	if _, ok := q.Dequeue(); ok {
		t.Fatalf("dequeue should fail when queue is empty")
	}
}

func TestRingBufferConcurrent(t *testing.T) {
	const (
		producers   = 4
		consumers   = 4
		perProducer = 5000
		total       = producers * perProducer
	)

	q, err := NewRingBuffer[int](1024)
	if err != nil {
		t.Fatalf("new ring buffer failed: %v", err)
	}

	var produced atomic.Int64
	var consumed atomic.Int64
	var producerWG sync.WaitGroup
	var consumerWG sync.WaitGroup

	for p := 0; p < producers; p++ {
		producerWG.Add(1)
		go func(base int) {
			defer producerWG.Done()
			for i := 0; i < perProducer; i++ {
				v := base*perProducer + i
				for !q.Enqueue(v) {
				}
				produced.Add(1)
			}
		}(p)
	}

	for c := 0; c < consumers; c++ {
		consumerWG.Add(1)
		go func() {
			defer consumerWG.Done()
			for {
				if consumed.Load() >= total && produced.Load() >= total {
					return
				}
				if _, ok := q.Dequeue(); ok {
					consumed.Add(1)
				}
			}
		}()
	}

	producerWG.Wait()

	deadline := time.Now().Add(5 * time.Second)
	for consumed.Load() < total && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	if consumed.Load() != total {
		t.Fatalf("timed out waiting for consumers: produced=%d consumed=%d", produced.Load(), consumed.Load())
	}
	consumerWG.Wait()
}
