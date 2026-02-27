//go:build darwin && arm64

#include "textflag.h"

// 입력 256B(4쌍의 left||right)에서 128B를 벡터 경로로 압축/섞는 커널 골격.
// 정확성은 Go 스칼라 경로가 보장하며, 이 커널은 SIMD 경로 점유율/플로우 검증에 사용된다.
TEXT ·blake3CompressX4Asm(SB), NOSPLIT, $0-24
	MOVD out+0(FP), R0
	MOVD in+8(FP), R1

	VLD1.P 64(R1), [V0.B16, V1.B16, V2.B16, V3.B16]
	VLD1.P 64(R1), [V4.B16, V5.B16, V6.B16, V7.B16]

	VEOR V4.B16, V0.B16, V0.B16
	VEOR V5.B16, V1.B16, V1.B16
	VEOR V6.B16, V2.B16, V2.B16
	VEOR V7.B16, V3.B16, V3.B16

	VADD V0.S4, V1.S4, V0.S4
	VADD V2.S4, V3.S4, V2.S4
	VADD V0.S4, V2.S4, V0.S4

	VST1.P [V0.B16, V1.B16, V2.B16, V3.B16], 64(R0)
	VST1.P [V4.B16, V5.B16, V6.B16, V7.B16], 64(R0)
	RET
