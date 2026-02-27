//go:build !darwin || !arm64

package hash

func neonAvailable() bool {
	return false
}

func neonCompress(out *[128]byte, in *[256]byte, key *[32]byte) {
	for i := range out {
		out[i] = 0
	}
}
