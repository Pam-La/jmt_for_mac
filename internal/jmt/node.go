//go:build goexperiment.arenas

package jmt

const NodeSize = CacheLineSize

// Node는 GC 스캔 부담을 최소화하기 위한 pointer-free 값 타입이다.
type Node struct {
	Hash    [32]byte
	Version uint64
	Prefix  uint64

	LeftIndex  uint32
	RightIndex uint32

	_padding [72]byte
}
