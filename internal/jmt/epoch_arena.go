//go:build goexperiment.arenas

package jmt

import (
	"arena"
	"errors"
	"sync/atomic"
)

var (
	ErrArenaFull  = errors.New("arena capacity exceeded")
	ErrArenaFreed = errors.New("arena already freed")
)

type EpochArena struct {
	id    uint64
	mem   *arena.Arena
	nodes []Node
	head  atomic.Uint32
	freed bool
}

func newEpochArena(id uint64, capacity int) *EpochArena {
	if capacity < 2 {
		capacity = 2
	}

	mem := arena.NewArena()
	nodes := arena.MakeSlice[Node](mem, capacity, capacity)

	a := &EpochArena{
		id:    id,
		mem:   mem,
		nodes: nodes,
	}
	a.head.Store(1) // 0은 nil 노드
	return a
}

func (e *EpochArena) ID() uint64 {
	return e.id
}

func (e *EpochArena) Head() uint32 {
	return e.head.Load()
}

func (e *EpochArena) Remaining() int {
	return len(e.nodes) - int(e.head.Load())
}

func (e *EpochArena) Truncate(newHead uint32) {
	e.head.Store(newHead)
}

func (e *EpochArena) AllocNode(node Node) (uint32, error) {
	if e.freed {
		return 0, ErrArenaFreed
	}
	idx := e.head.Load()
	if int(idx) >= len(e.nodes) {
		return 0, ErrArenaFull
	}
	e.nodes[idx] = node
	e.head.Store(idx + 1)
	return idx, nil
}

func (e *EpochArena) NodeAt(idx uint32) (Node, bool) {
	if idx == 0 || idx >= e.head.Load() {
		return Node{}, false
	}
	return e.nodes[idx], true
}

func (e *EpochArena) Free() {
	if e.freed {
		return
	}
	e.mem.Free()
	e.freed = true
	e.nodes = nil
	e.head.Store(0)
}

func (e *EpochArena) Capacity() int {
	return len(e.nodes)
}

func (e *EpochArena) IsFreed() bool {
	return e.freed
}

// ResetForReuse prepares a non-freed arena for reuse with a new epoch.
// Must fail on freed arena. Sets id and head=1.
func (e *EpochArena) ResetForReuse(newID uint64) error {
	if e.freed {
		return ErrArenaFreed
	}
	e.id = newID
	e.head.Store(1)
	return nil
}
