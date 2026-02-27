//go:build darwin && arm64

package hash

import "os"

var asmEnabled = os.Getenv("JMT_FORCE_ASM") == "1"

func neonAvailable() bool {
	return asmEnabled
}

//go:noescape
func blake3CompressX4Asm(out *[128]byte, in *[256]byte, key *[32]byte)

func neonCompress(out *[128]byte, in *[256]byte, key *[32]byte) {
	blake3CompressX4Asm(out, in, key)
}
