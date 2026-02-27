//go:build goexperiment.arenas

package jmt

import "bytes"

const (
	leafBitMask uint64 = 1 << 47
	depthShift  uint64 = 48
	depthMask   uint64 = 0xFFFF << depthShift
)

//go:inline
func makePrefix(depth uint16, leaf bool) uint64 {
	prefix := uint64(depth) << depthShift
	if leaf {
		prefix |= leafBitMask
	}
	return prefix
}

//go:inline
func decodeDepth(prefix uint64) uint16 {
	return uint16((prefix & depthMask) >> depthShift)
}

//go:inline
func isLeaf(prefix uint64) bool {
	return (prefix & leafBitMask) != 0
}

//go:inline
func samePrefix(a [32]byte, b [32]byte, depth uint16) bool {
	fullBytes := int(depth / 8)
	if fullBytes > 0 && !bytes.Equal(a[:fullBytes], b[:fullBytes]) {
		return false
	}
	remBits := depth % 8
	if remBits == 0 {
		return true
	}
	mask := byte(0xFF << (8 - remBits))
	return (a[fullBytes] & mask) == (b[fullBytes] & mask)
}

//go:inline
func bitAt(key [32]byte, depth uint16) uint8 {
	byteIndex := depth / 8
	bitOffset := 7 - (depth % 8)
	return (key[byteIndex] >> bitOffset) & 1
}

//go:inline
func prefixPath(key [32]byte, depth uint16) [32]byte {
	if depth >= JMTTreeDepth {
		return key
	}

	out := key
	fullBytes := int(depth / 8)
	remBits := depth % 8

	if remBits == 0 {
		for i := fullBytes; i < len(out); i++ {
			out[i] = 0
		}
		return out
	}

	mask := byte(0xFF << (8 - remBits))
	out[fullBytes] &= mask
	for i := fullBytes + 1; i < len(out); i++ {
		out[i] = 0
	}
	return out
}
