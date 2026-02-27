//go:build goexperiment.arenas

package jmt

// pathStack은 root->leaf down-pass에서 각 depth의 sibling 인덱스를 보관한다.
// sibling은 이전 버전 루트 기준 global node index다.
type pathStack struct {
	sibling [JMTTreeDepth]uint32
}

func (u *BatchUpdater) fillPathStack(t *StateTree, rootIndex uint32, key [32]byte, stack *pathStack) {
	current := rootIndex
	for depth := 0; depth < JMTTreeDepth; depth++ {
		if current == 0 {
			stack.sibling[depth] = 0
			continue
		}

		node, _, ok := t.nodeByIndex(current)
		if !ok {
			stack.sibling[depth] = 0
			current = 0
			continue
		}

		if bitAt(key, uint16(depth)) == 0 {
			stack.sibling[depth] = node.RightIndex
			current = node.LeftIndex
		} else {
			stack.sibling[depth] = node.LeftIndex
			current = node.RightIndex
		}
	}
}
