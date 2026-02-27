package proof

import "github.com/Pam-La/jmt_for_mac/internal/hash"

func Verify(engine *hash.Engine, key [32]byte, value [32]byte, proof MerkleProof, expectedRoot [32]byte) bool {
	if proof.Exists {
		leafHash := engine.HashLeaf(&key, &value)
		if leafHash != proof.LeafHash {
			return false
		}
		return verifyFromLeaf(engine, key, leafHash, proof, expectedRoot)
	}
	return verifyFromLeaf(engine, key, engine.ZeroHash(256), proof, expectedRoot)
}

func VerifyLeafHash(engine *hash.Engine, key [32]byte, leafHash [32]byte, proof MerkleProof, expectedRoot [32]byte) bool {
	if proof.Exists && leafHash != proof.LeafHash {
		return false
	}
	return verifyFromLeaf(engine, key, leafHash, proof, expectedRoot)
}

func verifyFromLeaf(engine *hash.Engine, key [32]byte, leafHash [32]byte, proof MerkleProof, expectedRoot [32]byte) bool {
	current := leafHash
	for depth := TreeDepth - 1; depth >= 0; depth-- {
		sibling := proof.Siblings[depth]
		bit := bitAt(key, uint16(depth))
		if bit == 0 {
			current = engine.HashParent(&current, &sibling)
		} else {
			current = engine.HashParent(&sibling, &current)
		}
	}
	return current == expectedRoot
}

func bitAt(key [32]byte, depth uint16) uint8 {
	byteIndex := depth / 8
	bitOffset := 7 - (depth % 8)
	return (key[byteIndex] >> bitOffset) & 1
}
