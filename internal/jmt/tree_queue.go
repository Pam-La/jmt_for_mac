//go:build goexperiment.arenas

package jmt

import (
	asyncq "github.com/Pam-La/jmt_for_mac/internal/async"
)

// DrainMutationQueue는 lock-free 링 버퍼에 쌓인 배치를 순서대로 반영한다.
// maxBatches <= 0이면 큐가 빌 때까지 처리한다.
func (t *StateTree) DrainMutationQueue(queue *asyncq.RingBuffer[[]Mutation], maxBatches int) (Snapshot, int, error) {
	var last Snapshot
	processed := 0

	for maxBatches <= 0 || processed < maxBatches {
		batch, ok := queue.Dequeue()
		if !ok {
			break
		}
		snapshot, err := t.ApplyBatch(batch)
		if err != nil {
			return Snapshot{}, processed, err
		}
		last = snapshot
		processed++
	}

	if processed == 0 {
		current := t.versions.latest.Load()
		if current == nil {
			return Snapshot{}, 0, ErrUnknownVersion
		}
		return *current, 0, nil
	}
	return last, processed, nil
}
