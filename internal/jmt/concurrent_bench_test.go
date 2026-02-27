//go:build goexperiment.arenas

package jmt

import "testing"

func BenchmarkJMTBatchCommit(b *testing.B) {
	tree := NewStateTree(Config{
		InitialArenaCapacity: 1 << 18,
		RetainVersions:       4,
	})
	defer tree.Close()

	mutations := make([]Mutation, 256)
	for i := 0; i < len(mutations); i++ {
		mutations[i] = Mutation{
			Key:   fixedWord(byte(i)),
			Value: fixedWord(byte(i + 1)),
		}
	}

	requiredPerBatch := uint64(estimateRequiredNodes(len(mutations)))
	estimatedNodes := (uint64(b.N) + 1) * requiredPerBatch
	maxNodes := uint64(maxNodeIndex - 1)
	if estimatedNodes > maxNodes {
		estimatedNodes = maxNodes
	}
	if err := tree.PreallocateLocatorChunks(uint32(estimatedNodes)); err != nil {
		b.Fatalf("preallocate locator chunks failed: %v", err)
	}
	if _, err := tree.ApplyBatch(mutations); err != nil {
		b.Fatalf("warmup apply failed: %v", err)
	}

	b.ReportAllocs()
	tree.Hasher().ResetStats()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := 0; j < len(mutations); j++ {
			value := fixedWord(byte((i + j) & 0xFF))
			mutations[j].Value = value
		}
		if _, err := tree.ApplyBatch(mutations); err != nil {
			b.Fatalf("apply failed: %v", err)
		}
	}
	b.StopTimer()
	stats := tree.HashStats()
	b.ReportMetric(stats.ParentSIMDRatio()*100, "simd_parent_pct")
	b.ReportMetric(float64(stats.ParentX4Batches), "parent_x4_batches")
	b.ReportMetric(float64(stats.ParentScalarCalls), "parent_scalar_calls")
	b.ReportMetric(float64(stats.ASMCalls), "asm_calls")
}

func BenchmarkJMTProofConcurrent(b *testing.B) {
	tree := NewStateTree(Config{
		InitialArenaCapacity: 1 << 18,
		RetainVersions:       8,
	})
	defer tree.Close()

	for i := 0; i < 1024; i++ {
		_, err := tree.ApplyBatch([]Mutation{{
			Key:   fixedWord(byte(i)),
			Value: fixedWord(byte(i + 5)),
		}})
		if err != nil {
			b.Fatalf("seed apply failed: %v", err)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		idx := 0
		for pb.Next() {
			key := fixedWord(byte(idx & 0xFF))
			txn := tree.AcquireLatest()
			_ = txn.GenerateProof(key)
			txn.Release()
			idx++
		}
	})
}
