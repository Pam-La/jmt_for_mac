//go:build goexperiment.arenas

package jmt

import (
	"encoding/binary"
	"reflect"
	"sync"
	"testing"
	"unsafe"

	asyncq "github.com/Pam-La/jmt_for_mac/internal/async"
	"github.com/Pam-La/jmt_for_mac/internal/proof"
)

func TestNodeLayoutIsPointerFree(t *testing.T) {
	if got := unsafe.Sizeof(Node{}); got != NodeSize {
		t.Fatalf("unexpected node size: got=%d want=%d", got, NodeSize)
	}

	nodeType := reflect.TypeOf(Node{})
	if hasForbiddenPointerKinds(nodeType) {
		t.Fatalf("node layout contains pointer-like fields")
	}
}

func TestApplyBatchAndVerifyProof(t *testing.T) {
	tree := NewStateTree(Config{
		InitialArenaCapacity: 1 << 14,
		RetainVersions:       32,
	})
	defer tree.Close()

	key := fixedWord(0x11)
	value := fixedWord(0x22)

	snap, err := tree.ApplyBatch([]Mutation{
		{Key: key, Value: value},
	})
	if err != nil {
		t.Fatalf("apply batch failed: %v", err)
	}
	if snap.Version != 1 {
		t.Fatalf("unexpected version: got=%d", snap.Version)
	}

	txn := tree.AcquireLatest()
	p := txn.GenerateProof(key)
	root := txn.RootHash()
	txn.Release()

	if !p.Exists {
		t.Fatalf("expected proof to exist")
	}
	if ok := proof.Verify(tree.hasher, key, value, p, root); !ok {
		t.Fatalf("proof verification failed")
	}
}

func TestRollbackRestoresPreviousVersion(t *testing.T) {
	tree := NewStateTree(Config{
		InitialArenaCapacity: 1 << 15,
		RetainVersions:       32,
	})
	defer tree.Close()

	keyA := fixedWord(0xA1)
	valA := fixedWord(0xB1)
	keyB := fixedWord(0xA2)
	valB := fixedWord(0xB2)

	if _, err := tree.ApplyBatch([]Mutation{{Key: keyA, Value: valA}}); err != nil {
		t.Fatalf("v1 apply failed: %v", err)
	}
	if _, err := tree.ApplyBatch([]Mutation{{Key: keyB, Value: valB}}); err != nil {
		t.Fatalf("v2 apply failed: %v", err)
	}

	rolled, err := tree.Rollback(1)
	if err != nil {
		t.Fatalf("rollback failed: %v", err)
	}
	if rolled.Version != 1 {
		t.Fatalf("unexpected rollback version: got=%d", rolled.Version)
	}

	txn := tree.AcquireLatest()
	defer txn.Release()

	proofA := txn.GenerateProof(keyA)
	if !proofA.Exists {
		t.Fatalf("expected keyA to exist after rollback")
	}
	if ok := proof.Verify(tree.hasher, keyA, valA, proofA, txn.RootHash()); !ok {
		t.Fatalf("keyA proof verification failed after rollback")
	}

	proofB := txn.GenerateProof(keyB)
	if proofB.Exists {
		t.Fatalf("expected keyB to be removed after rollback")
	}
}

func TestConcurrentReadersWithSingleWriter(t *testing.T) {
	tree := NewStateTree(Config{
		InitialArenaCapacity: 1 << 16,
		RetainVersions:       16,
	})
	defer tree.Close()

	for i := 0; i < 128; i++ {
		_, err := tree.ApplyBatch([]Mutation{{
			Key:   fixedWord(byte(i)),
			Value: fixedWord(byte(i + 1)),
		}})
		if err != nil {
			t.Fatalf("seed apply failed: %v", err)
		}
	}

	stop := make(chan struct{})
	var wg sync.WaitGroup

	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(readerID int) {
			defer wg.Done()
			key := fixedWord(byte(readerID))
			for {
				select {
				case <-stop:
					return
				default:
					txn := tree.AcquireLatest()
					_ = txn.GenerateProof(key)
					_ = txn.RootHash()
					txn.Release()
				}
			}
		}(i)
	}

	for i := 0; i < 24; i++ {
		_, err := tree.ApplyBatch([]Mutation{{
			Key:   fixedWord(byte(i)),
			Value: fixedWord(byte(i + 10)),
		}})
		if err != nil {
			close(stop)
			wg.Wait()
			t.Fatalf("writer apply failed: %v", err)
		}
	}

	close(stop)
	wg.Wait()
}

func TestDrainMutationQueue(t *testing.T) {
	tree := NewStateTree(Config{
		InitialArenaCapacity: 1 << 14,
		RetainVersions:       8,
	})
	defer tree.Close()

	queue, err := asyncq.NewRingBuffer[[]Mutation](8)
	if err != nil {
		t.Fatalf("queue init failed: %v", err)
	}

	keyA := fixedWord(0x61)
	valA := fixedWord(0x71)
	keyB := fixedWord(0x62)
	valB := fixedWord(0x72)

	if ok := queue.Enqueue([]Mutation{{Key: keyA, Value: valA}}); !ok {
		t.Fatalf("enqueue batch A failed")
	}
	if ok := queue.Enqueue([]Mutation{{Key: keyB, Value: valB}}); !ok {
		t.Fatalf("enqueue batch B failed")
	}

	snap, processed, err := tree.DrainMutationQueue(queue, 0)
	if err != nil {
		t.Fatalf("drain failed: %v", err)
	}
	if processed != 2 {
		t.Fatalf("unexpected processed batch count: got=%d want=2", processed)
	}
	if snap.Version != 2 {
		t.Fatalf("unexpected version after drain: got=%d want=2", snap.Version)
	}
}

func TestDuplicateKeyLastWriteWinsInBatch(t *testing.T) {
	tree := NewStateTree(Config{
		InitialArenaCapacity: 1 << 14,
		RetainVersions:       8,
	})
	defer tree.Close()

	key := fixedWord(0x44)
	oldValue := fixedWord(0x55)
	newValue := fixedWord(0x66)

	_, err := tree.ApplyBatch([]Mutation{
		{Key: key, Value: oldValue},
		{Key: key, Value: newValue},
	})
	if err != nil {
		t.Fatalf("apply with duplicate key failed: %v", err)
	}

	txn := tree.AcquireLatest()
	defer txn.Release()
	proofLatest := txn.GenerateProof(key)
	root := txn.RootHash()
	if !proof.Verify(tree.hasher, key, newValue, proofLatest, root) {
		t.Fatalf("latest value proof verification failed")
	}
	if proof.Verify(tree.hasher, key, oldValue, proofLatest, root) {
		t.Fatalf("old value should not verify after last-write-wins")
	}
}

func TestSparseUpdatePreservesUnchangedProof(t *testing.T) {
	tree := NewStateTree(Config{
		InitialArenaCapacity: 1 << 15,
		RetainVersions:       16,
	})
	defer tree.Close()

	keyA := fixedWord(0x71)
	valA := fixedWord(0x81)
	keyB := fixedWord(0x72)
	valB := fixedWord(0x82)
	valB2 := fixedWord(0x92)

	if _, err := tree.ApplyBatch([]Mutation{
		{Key: keyA, Value: valA},
		{Key: keyB, Value: valB},
	}); err != nil {
		t.Fatalf("initial apply failed: %v", err)
	}

	if _, err := tree.ApplyBatch([]Mutation{
		{Key: keyB, Value: valB2},
	}); err != nil {
		t.Fatalf("sparse apply failed: %v", err)
	}

	txn := tree.AcquireLatest()
	defer txn.Release()
	proofA := txn.GenerateProof(keyA)
	if !proofA.Exists {
		t.Fatalf("expected unchanged key to remain in tree")
	}
	if !proof.Verify(tree.hasher, keyA, valA, proofA, txn.RootHash()) {
		t.Fatalf("unchanged key proof verification failed")
	}
}

func fixedWord(seed byte) [32]byte {
	var out [32]byte
	for i := 0; i < len(out); i++ {
		out[i] = seed + byte(i)
	}
	return out
}

// keyFromUint32 returns a deterministic 32-byte key from a uint32 (big-endian in first 4 bytes).
func keyFromUint32(n uint32) [32]byte {
	var out [32]byte
	binary.BigEndian.PutUint32(out[:4], n)
	return out
}

func TestSIMDParentRatioLargeBatch(t *testing.T) {
	tree := NewStateTree(Config{
		InitialArenaCapacity: 1 << 20,
		RetainVersions:       8,
	})
	defer tree.Close()

	mutations := make([]Mutation, 10000)
	for i := 0; i < len(mutations); i++ {
		mutations[i] = Mutation{
			Key:   keyFromUint32(uint32(i)),
			Value: keyFromUint32(uint32(i + 1)),
		}
	}

	tree.Hasher().ResetStats()
	if _, err := tree.ApplyBatch(mutations); err != nil {
		t.Fatalf("apply batch failed: %v", err)
	}

	ratio := tree.ParentSIMDRatio()
	if ratio < 0.95 {
		t.Errorf("parent SIMD ratio = %.4f, want >= 0.95", ratio)
	}
}
