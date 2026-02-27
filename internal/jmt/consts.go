//go:build goexperiment.arenas

package jmt

// Tree topology and SIMD routing.
const (
	JMTTreeDepth  = 256
	SIMDChunkSize = 4
)

// Hardware and memory-layout assumptions.
const (
	CacheLineSize = 128

	LocatorChunkShift = 17
	LocatorChunkSize  = 1 << LocatorChunkShift
	LocatorChunkMask  = LocatorChunkSize - 1

	SnapshotRingSize = 1024
)

// Internal sizing and bounds.
const (
	minEpochRingSize          = 1024
	locatorDefaultTargetBytes = 16 << 30
	locatorEntrySizeBytes     = 8 // sizeof(nodeLocator)
	maxNodeIndex              = ^uint32(0)

	minInitialArenaCapacity = 1024
	defaultRetainVersions   = 8
	retainRingMultiplier    = 8

	warmPoolBootstrapCount = 3
	warmPoolMaxSize        = 8

	batchNodeEstimatePerMutation = 272
	batchNodeEstimateBase        = 2048
)
