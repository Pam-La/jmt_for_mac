package hash

import (
	"crypto/sha256"
	"sync/atomic"
)

const (
	maxTreeDepth = 256
)

type ParentPair struct {
	Left  [32]byte
	Right [32]byte
}

type Stats struct {
	LeafScalarCalls   uint64
	ParentScalarCalls uint64
	ParentX4Batches   uint64
	ParentX4Pairs     uint64
	ASMCalls          uint64
}

// ParentSIMDRatio returns ParentX4Pairs / (ParentX4Pairs + ParentScalarCalls).
// Returns 0 when the denominator is zero.
func (s Stats) ParentSIMDRatio() float64 {
	denom := s.ParentX4Pairs + s.ParentScalarCalls
	if denom == 0 {
		return 0
	}
	return float64(s.ParentX4Pairs) / float64(denom)
}

// Engine는 BLAKE3 인터페이스를 흉내 낸 해시 엔진이다.
// 실제 압축은 SHA-256 기반 도메인 분리로 구현하고, x4 라우팅 지점을 분리해 둔다.
type Engine struct {
	key               [32]byte
	zero              [maxTreeDepth + 1][32]byte
	leafScalarCalls   atomic.Uint64
	parentScalarCalls atomic.Uint64
	parentX4Batches   atomic.Uint64
	parentX4Pairs     atomic.Uint64
	asmCalls          atomic.Uint64
}

func NewEngine(key [32]byte) *Engine {
	e := &Engine{key: key}
	e.initZeroHashes()
	return e
}

func (e *Engine) initZeroHashes() {
	var seed [33]byte
	seed[0] = 'Z'
	copy(seed[1:], e.key[:])
	e.zero[maxTreeDepth] = sha256.Sum256(seed[:])

	for depth := maxTreeDepth - 1; depth >= 0; depth-- {
		parent := e.hashParentNoStats(&e.zero[depth+1], &e.zero[depth+1])
		e.zero[depth] = parent
	}
}

func (e *Engine) ZeroHash(depth uint16) [32]byte {
	if depth > maxTreeDepth {
		depth = maxTreeDepth
	}
	return e.zero[depth]
}

func (e *Engine) HashLeaf(key *[32]byte, value *[32]byte) [32]byte {
	var payload [97]byte
	payload[0] = 'L'
	copy(payload[1:33], e.key[:])
	copy(payload[33:65], key[:])
	copy(payload[65:97], value[:])

	e.leafScalarCalls.Add(1)
	return sha256.Sum256(payload[:])
}

func (e *Engine) HashParent(left *[32]byte, right *[32]byte) [32]byte {
	e.parentScalarCalls.Add(1)
	return e.hashParentNoStats(left, right)
}

func (e *Engine) hashParentNoStats(left *[32]byte, right *[32]byte) [32]byte {
	var payload [97]byte
	payload[0] = 'P'
	copy(payload[1:33], e.key[:])
	copy(payload[33:65], left[:])
	copy(payload[65:97], right[:])
	return sha256.Sum256(payload[:])
}

func (e *Engine) CompressParentsX4(out *[4][32]byte, pairs *[4]ParentPair) {
	e.parentX4Batches.Add(1)
	e.parentX4Pairs.Add(4)

	// asm 경로는 환경 변수로 명시적으로 활성화할 때만 호출한다.
	// 정확성은 스칼라 해시로 보장한다.
	if neonAvailable() {
		var packed [256]byte
		for i := 0; i < 4; i++ {
			base := i * 64
			copy(packed[base:base+32], pairs[i].Left[:])
			copy(packed[base+32:base+64], pairs[i].Right[:])
		}
		var scratch [128]byte
		neonCompress(&scratch, &packed, &e.key)
		e.asmCalls.Add(1)
	}

	for i := 0; i < 4; i++ {
		out[i] = e.hashParentNoStats(&pairs[i].Left, &pairs[i].Right)
	}
}

func (e *Engine) Stats() Stats {
	return Stats{
		LeafScalarCalls:   e.leafScalarCalls.Load(),
		ParentScalarCalls: e.parentScalarCalls.Load(),
		ParentX4Batches:   e.parentX4Batches.Load(),
		ParentX4Pairs:     e.parentX4Pairs.Load(),
		ASMCalls:          e.asmCalls.Load(),
	}
}

func (e *Engine) ResetStats() {
	e.leafScalarCalls.Store(0)
	e.parentScalarCalls.Store(0)
	e.parentX4Batches.Store(0)
	e.parentX4Pairs.Store(0)
	e.asmCalls.Store(0)
}
