//go:build goexperiment.arenas

package jmt

import "sync/atomic"

type MemoryManager struct {
	initialArenaCapacity int

	epochs      []*EpochArena
	epochByID   map[uint64]*EpochArena
	activeEpoch *EpochArena
	nextEpochID uint64

	epochRing []epochRingSlot

	warmPool    []*EpochArena
	maxPoolSize int

	nextLocator  uint32
	locatorStore atomic.Pointer[locatorStore]
}

type VersionControl struct {
	retainVersions uint64

	activeReaders atomic.Int64
	latest        atomic.Pointer[Snapshot]
	snapshotRing  [SnapshotRingSize]Snapshot

	versionRoots  map[uint64]rootRef
	epochRefcount map[uint64]int
}

type BatchUpdater struct {
	dirtyQueue dirtyQueue
	pathStacks []pathStack
	levelBuf   levelBuffer
}

func newMemoryManager(initialArenaCapacity int, retainVersions uint64, initialEpoch *EpochArena) MemoryManager {
	ringSize := computeEpochRingSize(retainVersions)
	ring := make([]epochRingSlot, ringSize)
	slotIdx := initialEpoch.ID() % uint64(len(ring))
	ring[slotIdx].arena.Store(initialEpoch)
	ring[slotIdx].epochID.Store(initialEpoch.ID())

	warmPool := make([]*EpochArena, 0, warmPoolMaxSize)
	for i := 0; i < warmPoolBootstrapCount; i++ {
		warmPool = append(warmPool, newEpochArena(0, initialArenaCapacity))
	}

	dirSize := computeDefaultLocatorDirSize()
	chunkSlots := make([]atomic.Pointer[locatorChunk], dirSize)
	chunkSlots[0].Store(&locatorChunk{})
	store := locatorStore{chunks: chunkSlots}

	m := MemoryManager{
		initialArenaCapacity: initialArenaCapacity,
		epochs:               []*EpochArena{initialEpoch},
		epochByID:            map[uint64]*EpochArena{initialEpoch.ID(): initialEpoch},
		activeEpoch:          initialEpoch,
		nextEpochID:          2,
		epochRing:            ring,
		warmPool:             warmPool,
		maxPoolSize:          warmPoolMaxSize,
		nextLocator:          1,
	}
	m.locatorStore.Store(&store)
	return m
}

func newVersionControl(retainVersions uint64, initialEpoch *EpochArena, root [32]byte) VersionControl {
	vc := VersionControl{
		retainVersions: retainVersions,
		versionRoots: map[uint64]rootRef{
			0: {
				epochID:   initialEpoch.ID(),
				rootIndex: 0,
				rootHash:  root,
			},
		},
		epochRefcount: map[uint64]int{initialEpoch.ID(): 1},
	}
	snap := &vc.snapshotRing[0]
	*snap = Snapshot{
		Version:   0,
		EpochID:   initialEpoch.ID(),
		RootIndex: 0,
		RootHash:  root,
	}
	vc.latest.Store(snap)
	return vc
}

func newBatchUpdater() BatchUpdater {
	return BatchUpdater{}
}
