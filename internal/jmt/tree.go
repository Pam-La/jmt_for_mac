//go:build goexperiment.arenas

package jmt

import (
	"errors"
	"sync"
	"sync/atomic"

	"github.com/Pam-La/jmt_for_mac/internal/hash"
)

var (
	ErrUnknownVersion   = errors.New("unknown version")
	ErrNodeIndexExhaust = errors.New("global node index exhausted")
	ErrEpochIDOverflow  = errors.New("epoch ID exceeds uint32")
)

type Config struct {
	InitialArenaCapacity int
	RetainVersions       uint64
	HashKey              [32]byte
}

type Snapshot struct {
	Version   uint64
	EpochID   uint64
	RootIndex uint32
	RootHash  [32]byte
}

type rootRef struct {
	epochID   uint64
	rootIndex uint32
	rootHash  [32]byte
}

type nodeLocator struct {
	epochID    uint32
	localIndex uint32
}

type locatorChunk [LocatorChunkSize]nodeLocator

type locatorStore struct {
	chunks []atomic.Pointer[locatorChunk]
}

type epochRingSlot struct {
	epochID atomic.Uint64
	arena   atomic.Pointer[EpochArena]
}

type StateTree struct {
	writerMu sync.Mutex

	hasher *hash.Engine

	memory   MemoryManager
	versions VersionControl
	updater  BatchUpdater
}

func NewStateTree(cfg Config) *StateTree {
	initial := cfg.InitialArenaCapacity
	if initial < minInitialArenaCapacity {
		initial = minInitialArenaCapacity
	}
	retain := cfg.RetainVersions
	if retain == 0 {
		retain = defaultRetainVersions
	}

	engine := hash.NewEngine(cfg.HashKey)
	initialEpoch := newEpochArena(1, initial)
	root := engine.ZeroHash(0)

	return &StateTree{
		hasher:   engine,
		memory:   newMemoryManager(initial, retain, initialEpoch),
		versions: newVersionControl(retain, initialEpoch, root),
		updater:  newBatchUpdater(),
	}
}

func (t *StateTree) LatestVersion() uint64 {
	snap := t.versions.latest.Load()
	if snap == nil {
		return 0
	}
	return snap.Version
}

func (t *StateTree) HashStats() hash.Stats {
	return t.hasher.Stats()
}

// ParentSIMDRatio returns the fraction of parent hash work that went through
// the batched X4 path, derived from hash stats. Returns 0 if no parent work.
func (t *StateTree) ParentSIMDRatio() float64 {
	return t.hasher.Stats().ParentSIMDRatio()
}

func (t *StateTree) Hasher() *hash.Engine {
	return t.hasher
}

func (t *StateTree) Close() {
	t.writerMu.Lock()
	defer t.writerMu.Unlock()

	for _, epoch := range t.memory.epochs {
		epoch.Free()
	}
	t.memory.epochs = nil
	t.memory.epochByID = nil
	t.memory.activeEpoch = nil
	t.versions.versionRoots = nil
	t.versions.epochRefcount = nil
	for i := range t.memory.warmPool {
		t.memory.warmPool[i].Free()
	}
	t.memory.warmPool = nil
	t.memory.locatorStore.Store(nil)
	t.memory.nextLocator = 0
}
