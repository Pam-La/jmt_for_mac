//go:build !darwin || !arm64

package jmt

// 엄격 모드: Apple Silicon 전용.
var _ = requiresDarwinArm64Target
