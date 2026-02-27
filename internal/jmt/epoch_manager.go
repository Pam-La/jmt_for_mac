//go:build goexperiment.arenas

package jmt

import (
	"errors"
	"fmt"
	"math"
)

func computeEpochRingSize(retain uint64) int {
	minSize := minEpochRingSize
	if r := int(retain * retainRingMultiplier); r > minSize {
		minSize = r
	}
	size := minEpochRingSize
	for size < minSize {
		size *= 2
	}
	return size
}

// computeDefaultLocatorDirSize returns the default locator directory size for a
// 16GB target (based on locator chunk size and nodeLocator size), minimum 1024.
func computeDefaultLocatorDirSize() int {
	chunkBytes := LocatorChunkSize * locatorEntrySizeBytes
	n := locatorDefaultTargetBytes / chunkBytes
	if n < minEpochRingSize {
		return minEpochRingSize
	}
	return int(n)
}

// acquireEpoch returns a fresh epoch for a new version.
// Caller must hold writerMu.
func (t *StateTree) acquireEpoch(capacity int) (*EpochArena, error) {
	if capacity < 2 {
		capacity = 2
	}
	if uint32(capacity) == 0 {
		return nil, fmt.Errorf("invalid epoch capacity: %d", capacity)
	}

	ep := t.takeFromPoolOrAlloc(capacity)
	t.memory.epochs = append(t.memory.epochs, ep)
	t.memory.epochByID[ep.ID()] = ep

	slotIdx := ep.ID() % uint64(len(t.memory.epochRing))
	t.memory.epochRing[slotIdx].arena.Store(ep)
	t.memory.epochRing[slotIdx].epochID.Store(ep.ID())

	return ep, nil
}

func (t *StateTree) takeFromPoolOrAlloc(capacity int) *EpochArena {
	if capacity < t.memory.initialArenaCapacity {
		capacity = t.memory.initialArenaCapacity
	}
	if n := len(t.memory.warmPool); n > 0 {
		ep := t.memory.warmPool[n-1]
		if ep.Capacity() >= capacity {
			t.memory.warmPool = t.memory.warmPool[:n-1]
			_ = ep.ResetForReuse(t.memory.nextEpochID)
			t.memory.nextEpochID++
			return ep
		}
	}
	ep := newEpochArena(t.memory.nextEpochID, capacity)
	t.memory.nextEpochID++
	return ep
}

func (t *StateTree) discardEpoch(epoch *EpochArena) {
	if epoch == nil {
		return
	}
	if t.memory.activeEpoch == epoch {
		t.memory.activeEpoch = nil
	}
	delete(t.memory.epochByID, epoch.ID())
	for i := len(t.memory.epochs) - 1; i >= 0; i-- {
		if t.memory.epochs[i] != epoch {
			continue
		}
		t.memory.epochs = append(t.memory.epochs[:i], t.memory.epochs[i+1:]...)
		break
	}
	slotIdx := epoch.ID() % uint64(len(t.memory.epochRing))
	if t.memory.epochRing[slotIdx].epochID.Load() == epoch.ID() {
		t.memory.epochRing[slotIdx].epochID.Store(0)
		t.memory.epochRing[slotIdx].arena.Store(nil)
	}
	epoch.Free()
}

func (t *StateTree) reserveLocatorSpace(extra uint32) error {
	if extra == 0 {
		return nil
	}
	next := t.memory.nextLocator
	if next+extra > maxNodeIndex || next+extra < next {
		return ErrNodeIndexExhaust
	}
	store := t.memory.locatorStore.Load()
	if store == nil {
		return ErrNodeIndexExhaust
	}
	lastIndex := next + extra - 1
	lastChunkIndex := int(lastIndex >> LocatorChunkShift)
	if lastChunkIndex >= len(store.chunks) {
		return ErrNodeIndexExhaust
	}
	for i := 0; i <= lastChunkIndex; i++ {
		if store.chunks[i].Load() == nil {
			store.chunks[i].Store(&locatorChunk{})
		}
	}
	return nil
}

// PreallocateLocatorChunks allocates locator chunks in advance so benchmark
// timers can focus on commit logic instead of first-touch chunk growth.
func (t *StateTree) PreallocateLocatorChunks(nodes uint32) error {
	t.writerMu.Lock()
	defer t.writerMu.Unlock()
	return t.reserveLocatorSpace(nodes)
}

func (t *StateTree) allocNode(epoch *EpochArena, node Node) (uint32, error) {
	if epoch == nil {
		return 0, errors.New("nil epoch")
	}
	localIndex, err := epoch.AllocNode(node)
	if err != nil {
		return 0, err
	}

	id := t.memory.nextLocator
	if id == 0 || id == maxNodeIndex {
		return 0, ErrNodeIndexExhaust
	}

	store := t.memory.locatorStore.Load()
	if store == nil {
		return 0, ErrNodeIndexExhaust
	}

	chunkIndex := int(id >> LocatorChunkShift)
	offset := id & LocatorChunkMask

	if chunkIndex >= len(store.chunks) {
		return 0, ErrNodeIndexExhaust
	}
	chunk := store.chunks[chunkIndex].Load()
	if chunk == nil {
		chunk = &locatorChunk{}
		store.chunks[chunkIndex].Store(chunk)
	}

	epochID := epoch.ID()
	if epochID > math.MaxUint32 {
		return 0, ErrEpochIDOverflow
	}
	chunk[offset] = nodeLocator{
		epochID:    uint32(epochID),
		localIndex: localIndex,
	}
	t.memory.nextLocator = id + 1
	return id, nil
}

func (t *StateTree) nodeByIndex(index uint32) (Node, *EpochArena, bool) {
	if index == 0 {
		return Node{}, nil, false
	}
	store := t.memory.locatorStore.Load()
	if store == nil {
		return Node{}, nil, false
	}
	chunkIndex := int(index >> LocatorChunkShift)
	if chunkIndex >= len(store.chunks) {
		return Node{}, nil, false
	}
	chunk := store.chunks[chunkIndex].Load()
	if chunk == nil {
		return Node{}, nil, false
	}
	loc := chunk[index&LocatorChunkMask]
	if loc.localIndex == 0 {
		return Node{}, nil, false
	}

	ring := t.memory.epochRing
	if len(ring) == 0 {
		return Node{}, nil, false
	}
	slotIdx := uint64(loc.epochID) % uint64(len(ring))
	slotEpochID := ring[slotIdx].epochID.Load()
	if slotEpochID != uint64(loc.epochID) {
		return Node{}, nil, false
	}
	epoch := ring[slotIdx].arena.Load()
	if epoch == nil || epoch.IsFreed() {
		return Node{}, nil, false
	}
	if epoch.ID() != slotEpochID {
		return Node{}, nil, false
	}
	if ring[slotIdx].epochID.Load() != slotEpochID {
		return Node{}, nil, false
	}

	node, ok := epoch.NodeAt(loc.localIndex)
	if !ok {
		return Node{}, nil, false
	}
	return node, epoch, true
}

func (t *StateTree) nodeHashAtDepth(index uint32, depth uint16) [32]byte {
	if index == 0 {
		return t.hasher.ZeroHash(depth)
	}
	node, _, ok := t.nodeByIndex(index)
	if !ok {
		return t.hasher.ZeroHash(depth)
	}
	return node.Hash
}
