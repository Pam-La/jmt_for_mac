//go:build goexperiment.arenas

package jmt

// writableSnapshotSlot returns a reusable ring slot when there are no active
// readers. If readers are active, it falls back to heap allocation so published
// snapshots are never overwritten while a read transaction can still observe
// them.
func (t *StateTree) writableSnapshotSlot(version uint64) *Snapshot {
	if t.versions.activeReaders.Load() > 0 {
		return &Snapshot{}
	}
	return &t.versions.snapshotRing[version%SnapshotRingSize]
}

func (t *StateTree) reclaimLocked() {
	if t.versions.activeReaders.Load() > 0 {
		return
	}
	latest := t.versions.latest.Load()
	if latest == nil {
		return
	}

	minKeep := uint64(0)
	if latest.Version > t.versions.retainVersions {
		minKeep = latest.Version - t.versions.retainVersions
	}

	for version := range t.versions.versionRoots {
		if version == 0 || version >= minKeep {
			continue
		}
		ref := t.versions.versionRoots[version]
		delete(t.versions.versionRoots, version)
		if cnt, ok := t.versions.epochRefcount[ref.epochID]; ok {
			cnt--
			t.versions.epochRefcount[ref.epochID] = cnt
			if cnt == 0 {
				t.recycleEpochLocked(ref.epochID)
			}
		}
	}
}

func (t *StateTree) recycleEpochLocked(epochID uint64) {
	ep, ok := t.memory.epochByID[epochID]
	if !ok {
		return
	}
	if t.memory.activeEpoch == ep {
		return
	}
	if epochID == 1 {
		return
	}
	delete(t.memory.epochByID, epochID)
	for i := len(t.memory.epochs) - 1; i >= 0; i-- {
		if t.memory.epochs[i] != ep {
			continue
		}
		t.memory.epochs = append(t.memory.epochs[:i], t.memory.epochs[i+1:]...)
		break
	}
	delete(t.versions.epochRefcount, epochID)
	slotIdx := epochID % uint64(len(t.memory.epochRing))
	if t.memory.epochRing[slotIdx].epochID.Load() == epochID {
		t.memory.epochRing[slotIdx].epochID.Store(0)
		t.memory.epochRing[slotIdx].arena.Store(nil)
	}
	if len(t.memory.warmPool) < t.memory.maxPoolSize {
		_ = ep.ResetForReuse(0)
		t.memory.warmPool = append(t.memory.warmPool, ep)
	} else {
		ep.Free()
	}
}
