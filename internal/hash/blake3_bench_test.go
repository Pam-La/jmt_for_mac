package hash

import "testing"

func BenchmarkBlake3ScalarParent(b *testing.B) {
	engine := NewEngine([32]byte{1, 2, 3})
	left := seededWord(0x31)
	right := seededWord(0x71)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = engine.HashParent(&left, &right)
	}
}

func BenchmarkBlake3X4(b *testing.B) {
	engine := NewEngine([32]byte{1, 2, 3})
	var pairs [4]ParentPair
	for i := 0; i < 4; i++ {
		pairs[i] = ParentPair{
			Left:  seededWord(byte(i + 1)),
			Right: seededWord(byte(i + 9)),
		}
	}

	var out [4][32]byte
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		engine.CompressParentsX4(&out, &pairs)
	}
}

func seededWord(seed byte) [32]byte {
	var out [32]byte
	for i := 0; i < 32; i++ {
		out[i] = seed + byte(i)
	}
	return out
}
