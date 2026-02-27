//go:build goexperiment.arenas

package jmt

type Mutation struct {
	Key    [32]byte
	Value  [32]byte
	Delete bool
}

type pendingParent struct {
	leftIndex  uint32
	rightIndex uint32
	key        [32]byte
	witness    uint32
}

func (t *StateTree) ApplyBatch(mutations []Mutation) (Snapshot, error) {
	t.writerMu.Lock()
	defer t.writerMu.Unlock()

	current := t.versions.latest.Load()
	if current == nil {
		return Snapshot{}, ErrUnknownVersion
	}
	if len(mutations) == 0 {
		return *current, nil
	}

	normalized := t.updater.dirtyQueue.normalize(mutations)
	if len(normalized) == 0 {
		return *current, nil
	}

	nextVersion := current.Version + 1
	requiredNodes := estimateRequiredNodes(len(normalized))

	var epoch *EpochArena
	prevActive := t.memory.activeEpoch
	createdEpoch := false
	if t.memory.activeEpoch != nil && t.memory.activeEpoch.Remaining() >= requiredNodes {
		epoch = t.memory.activeEpoch
	} else {
		allocCapacity := maxInt(requiredNodes, t.memory.initialArenaCapacity)
		var err error
		epoch, err = t.acquireEpoch(allocCapacity)
		if err != nil {
			return Snapshot{}, err
		}
		t.memory.activeEpoch = epoch
		createdEpoch = true
	}

	headBase := epoch.Head()
	locatorBase := t.memory.nextLocator

	if err := t.reserveLocatorSpace(uint32(requiredNodes)); err != nil {
		if createdEpoch {
			t.discardEpoch(epoch)
			t.memory.activeEpoch = prevActive
		}
		return Snapshot{}, err
	}

	rootIndex, rootHash, err := t.updater.applyDirtyPaths(t, current.RootIndex, epoch, nextVersion, normalized)
	if err != nil {
		t.memory.nextLocator = locatorBase
		if createdEpoch {
			t.discardEpoch(epoch)
			t.memory.activeEpoch = prevActive
		} else {
			epoch.Truncate(headBase)
		}
		return Snapshot{}, err
	}
	snapshot := t.writableSnapshotSlot(nextVersion)
	*snapshot = Snapshot{
		Version:   nextVersion,
		EpochID:   epoch.ID(),
		RootIndex: rootIndex,
		RootHash:  rootHash,
	}

	t.versions.latest.Store(snapshot)
	t.versions.versionRoots[nextVersion] = rootRef{
		epochID:   epoch.ID(),
		rootIndex: rootIndex,
		rootHash:  rootHash,
	}
	t.versions.epochRefcount[epoch.ID()]++
	t.reclaimLocked()

	return *snapshot, nil
}

func (t *StateTree) Rollback(version uint64) (Snapshot, error) {
	t.writerMu.Lock()
	defer t.writerMu.Unlock()

	ref, ok := t.versions.versionRoots[version]
	if !ok {
		return Snapshot{}, ErrUnknownVersion
	}
	if _, ok := t.memory.epochByID[ref.epochID]; !ok {
		return Snapshot{}, ErrUnknownVersion
	}
	snapshot := t.writableSnapshotSlot(version)
	*snapshot = Snapshot{
		Version:   version,
		EpochID:   ref.epochID,
		RootIndex: ref.rootIndex,
		RootHash:  ref.rootHash,
	}
	t.versions.latest.Store(snapshot)
	t.reclaimLocked()

	return *snapshot, nil
}

func estimateRequiredNodes(mutations int) int {
	estimated := (mutations * batchNodeEstimatePerMutation) + batchNodeEstimateBase
	if estimated > int(maxNodeIndex)-1 {
		estimated = int(maxNodeIndex) - 1
	}
	if estimated < 2 {
		return 2
	}
	return estimated
}

func maxInt(a, b int) int {
	if a >= b {
		return a
	}
	return b
}

func (u *BatchUpdater) applyDirtyPaths(t *StateTree, baseRoot uint32, epoch *EpochArena, version uint64, mutations []Mutation) (uint32, [32]byte, error) {
	if len(mutations) == 0 {
		return baseRoot, t.nodeHashAtDepth(baseRoot, 0), nil
	}

	if cap(u.pathStacks) < len(mutations) {
		u.pathStacks = make([]pathStack, len(mutations))
	}
	stacks := u.pathStacks[:len(mutations)]

	u.levelBuf.ensure(len(mutations))
	u.levelBuf.reset()
	curr := u.levelBuf.curr

	for i := range mutations {
		mutation := mutations[i]
		u.fillPathStack(t, baseRoot, mutation.Key, &stacks[i])

		leafIndex := uint32(0)
		if !mutation.Delete {
			leafHash := t.hasher.HashLeaf(&mutation.Key, &mutation.Value)
			leafNode := Node{
				Hash:    leafHash,
				Version: version,
				Prefix:  makePrefix(JMTTreeDepth, true),
			}
			var err error
			leafIndex, err = t.allocNode(epoch, leafNode)
			if err != nil {
				return 0, [32]byte{}, err
			}
		}

		curr = append(curr, levelEntry{
			key:     mutation.Key,
			index:   leafIndex,
			witness: uint32(i),
		})
	}

	u.levelBuf.curr = curr

	router := newSIMDRouter(t.hasher)
	for depth := JMTTreeDepth - 1; depth >= 0; depth-- {
		next := u.levelBuf.next[:0]
		router.Reset()

		i := 0
		for i < len(u.levelBuf.curr) {
			groupStart := i
			i++
			for i < len(u.levelBuf.curr) && samePrefix(u.levelBuf.curr[groupStart].key, u.levelBuf.curr[i].key, uint16(depth)) {
				i++
			}

			var (
				hasLeft, hasRight     bool
				leftIndex, rightIndex uint32
				leftWitness, rightWit uint32
			)

			for j := groupStart; j < i; j++ {
				entry := u.levelBuf.curr[j]
				if bitAt(entry.key, uint16(depth)) == 0 {
					hasLeft = true
					leftIndex = entry.index
					leftWitness = entry.witness
				} else {
					hasRight = true
					rightIndex = entry.index
					rightWit = entry.witness
				}
			}

			witness := leftWitness
			if !hasLeft {
				witness = rightWit
				leftIndex = stacks[rightWit].sibling[depth]
			}
			if !hasRight {
				rightIndex = stacks[leftWitness].sibling[depth]
			}

			parentKey := prefixPath(u.levelBuf.curr[groupStart].key, uint16(depth))
			if leftIndex == 0 && rightIndex == 0 {
				next = append(next, levelEntry{
					key:     parentKey,
					index:   0,
					witness: witness,
				})
				continue
			}

			meta := pendingParent{
				leftIndex:  leftIndex,
				rightIndex: rightIndex,
				key:        parentKey,
				witness:    witness,
			}
			full := router.Add(
				t.nodeHashAtDepth(leftIndex, uint16(depth+1)),
				t.nodeHashAtDepth(rightIndex, uint16(depth+1)),
				meta,
			)
			if full {
				hashes, metas := router.FlushX4()
				for k := 0; k < SIMDChunkSize; k++ {
					parentNode := Node{
						Hash:       hashes[k],
						Version:    version,
						Prefix:     makePrefix(uint16(depth), false),
						LeftIndex:  metas[k].leftIndex,
						RightIndex: metas[k].rightIndex,
					}
					parentIndex, err := t.allocNode(epoch, parentNode)
					if err != nil {
						return 0, [32]byte{}, err
					}
					next = append(next, levelEntry{
						key:     metas[k].key,
						index:   parentIndex,
						witness: metas[k].witness,
					})
				}
			}
		}

		for j := 0; j < router.count; j++ {
			pair := router.batch[j]
			meta := router.meta[j]
			parentHash := t.hasher.HashParent(&pair.Left, &pair.Right)
			parentNode := Node{
				Hash:       parentHash,
				Version:    version,
				Prefix:     makePrefix(uint16(depth), false),
				LeftIndex:  meta.leftIndex,
				RightIndex: meta.rightIndex,
			}
			parentIndex, err := t.allocNode(epoch, parentNode)
			if err != nil {
				return 0, [32]byte{}, err
			}
			next = append(next, levelEntry{
				key:     meta.key,
				index:   parentIndex,
				witness: meta.witness,
			})
		}
		router.Reset()

		u.levelBuf.next = next
		u.levelBuf.swap()
	}

	if len(u.levelBuf.curr) == 0 {
		return 0, t.hasher.ZeroHash(0), nil
	}
	rootIndex := u.levelBuf.curr[0].index
	rootHash := t.nodeHashAtDepth(rootIndex, 0)
	return rootIndex, rootHash, nil
}
