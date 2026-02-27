//go:build goexperiment.arenas

package jmt

import "github.com/Pam-La/jmt_for_mac/internal/hash"

// SIMDRouter batches parent pairs and routes full chunks to the x4 path.
type SIMDRouter struct {
	hasher *hash.Engine
	batch  [SIMDChunkSize]hash.ParentPair
	meta   [SIMDChunkSize]pendingParent
	out    [SIMDChunkSize][32]byte
	count  int
}

//go:inline
func newSIMDRouter(hasher *hash.Engine) SIMDRouter {
	return SIMDRouter{hasher: hasher}
}

//go:inline
func (r *SIMDRouter) Add(left [32]byte, right [32]byte, meta pendingParent) bool {
	idx := r.count
	r.batch[idx] = hash.ParentPair{Left: left, Right: right}
	r.meta[idx] = meta
	r.count++
	return r.count == SIMDChunkSize
}

func (r *SIMDRouter) FlushX4() (*[SIMDChunkSize][32]byte, *[SIMDChunkSize]pendingParent) {
	if r.count != SIMDChunkSize {
		return nil, nil
	}
	r.hasher.CompressParentsX4(&r.out, &r.batch)
	r.count = 0
	return &r.out, &r.meta
}

//go:inline
func (r *SIMDRouter) Reset() {
	r.count = 0
}
