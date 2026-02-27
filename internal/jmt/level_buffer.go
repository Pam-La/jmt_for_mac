//go:build goexperiment.arenas

package jmt

type levelEntry struct {
	key     [32]byte
	index   uint32
	witness uint32
}

type levelBuffer struct {
	curr []levelEntry
	next []levelEntry
}

func (b *levelBuffer) ensure(capacity int) {
	if capacity < 1 {
		capacity = 1
	}
	if cap(b.curr) < capacity {
		b.curr = make([]levelEntry, 0, capacity)
	} else {
		b.curr = b.curr[:0]
	}
	if cap(b.next) < capacity {
		b.next = make([]levelEntry, 0, capacity)
	} else {
		b.next = b.next[:0]
	}
}

func (b *levelBuffer) reset() {
	b.curr = b.curr[:0]
	b.next = b.next[:0]
}

func (b *levelBuffer) swap() {
	b.curr, b.next = b.next, b.curr
	b.next = b.next[:0]
}
