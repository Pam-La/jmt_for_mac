//go:build !goexperiment.arenas

package jmt

// 엄격 모드: GOEXPERIMENT=arenas 없이 빌드되면 컴파일 실패.
var _ = requiresGOEXPERIMENTArenas
