//go:build goexperiment.arenas

package jmt

import "github.com/Pam-La/jmt_for_mac/internal/proof"

type ReadTxn struct {
	tree     *StateTree
	snapshot *Snapshot
}

func (t *StateTree) AcquireLatest() ReadTxn {
	t.versions.activeReaders.Add(1)
	snap := t.versions.latest.Load()
	if snap == nil {
		t.versions.activeReaders.Add(-1)
		return ReadTxn{}
	}
	return ReadTxn{
		tree:     t,
		snapshot: snap,
	}
}

func (r ReadTxn) Release() {
	if r.tree == nil {
		return
	}
	r.tree.versions.activeReaders.Add(-1)
}

func (r ReadTxn) Snapshot() Snapshot {
	if r.snapshot == nil {
		return Snapshot{}
	}
	return *r.snapshot
}

func (r ReadTxn) RootHash() [32]byte {
	if r.snapshot == nil {
		return [32]byte{}
	}
	return r.snapshot.RootHash
}

func (r ReadTxn) GenerateProof(key [32]byte) proof.MerkleProof {
	if r.snapshot == nil {
		return proof.MerkleProof{}
	}

	var merkleProof proof.MerkleProof
	merkleProof.Version = r.snapshot.Version

	current := r.snapshot.RootIndex
	for depth := 0; depth < proof.TreeDepth; depth++ {
		if current == 0 {
			merkleProof.Siblings[depth] = r.tree.hasher.ZeroHash(uint16(depth + 1))
			continue
		}

		node, _, ok := r.tree.nodeByIndex(current)
		if !ok {
			merkleProof.Siblings[depth] = r.tree.hasher.ZeroHash(uint16(depth + 1))
			current = 0
			continue
		}

		bit := bitAt(key, uint16(depth))
		if bit == 0 {
			merkleProof.Siblings[depth] = r.tree.nodeHashAtDepth(node.RightIndex, uint16(depth+1))
			current = node.LeftIndex
		} else {
			merkleProof.Siblings[depth] = r.tree.nodeHashAtDepth(node.LeftIndex, uint16(depth+1))
			current = node.RightIndex
		}
	}

	if current != 0 {
		leaf, _, ok := r.tree.nodeByIndex(current)
		if ok && isLeaf(leaf.Prefix) && decodeDepth(leaf.Prefix) == uint16(JMTTreeDepth) {
			merkleProof.Exists = true
			merkleProof.LeafHash = leaf.Hash
			return merkleProof
		}
	}
	merkleProof.Exists = false
	merkleProof.LeafHash = r.tree.hasher.ZeroHash(JMTTreeDepth)
	return merkleProof
}

func (t *StateTree) RootHash() [32]byte {
	snap := t.versions.latest.Load()
	if snap == nil {
		return t.hasher.ZeroHash(0)
	}
	return snap.RootHash
}

func (t *StateTree) GenerateProofLatest(key [32]byte) proof.MerkleProof {
	txn := t.AcquireLatest()
	defer txn.Release()
	return txn.GenerateProof(key)
}

func (t *StateTree) SnapshotByVersion(version uint64) (Snapshot, error) {
	t.writerMu.Lock()
	defer t.writerMu.Unlock()

	ref, ok := t.versions.versionRoots[version]
	if !ok {
		return Snapshot{}, ErrUnknownVersion
	}
	if _, ok := t.memory.epochByID[ref.epochID]; !ok {
		return Snapshot{}, ErrUnknownVersion
	}
	return Snapshot{
		Version:   version,
		EpochID:   ref.epochID,
		RootIndex: ref.rootIndex,
		RootHash:  ref.rootHash,
	}, nil
}
