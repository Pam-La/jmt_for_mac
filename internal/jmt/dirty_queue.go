//go:build goexperiment.arenas

package jmt

import (
	"bytes"
	"slices"
)

type queuedMutation struct {
	mutation Mutation
	seq      int
}

// dirtyQueue는 배치 mutation을 key 기준으로 정렬하고 중복 key를 마지막 값으로 축약한다.
type dirtyQueue struct {
	staged []queuedMutation
	output []Mutation
}

func (q *dirtyQueue) normalize(mutations []Mutation) []Mutation {
	q.staged = q.staged[:0]
	q.output = q.output[:0]
	if len(mutations) == 0 {
		return q.output
	}

	for i := range mutations {
		q.staged = append(q.staged, queuedMutation{
			mutation: mutations[i],
			seq:      i,
		})
	}

	slices.SortFunc(q.staged, func(a, b queuedMutation) int {
		if cmp := bytes.Compare(a.mutation.Key[:], b.mutation.Key[:]); cmp != 0 {
			return cmp
		}
		switch {
		case a.seq < b.seq:
			return -1
		case a.seq > b.seq:
			return 1
		default:
			return 0
		}
	})

	for i := 0; i < len(q.staged); {
		j := i + 1
		for j < len(q.staged) && q.staged[j].mutation.Key == q.staged[i].mutation.Key {
			j++
		}
		q.output = append(q.output, q.staged[j-1].mutation)
		i = j
	}

	return q.output
}
