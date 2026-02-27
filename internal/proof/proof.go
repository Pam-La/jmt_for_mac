package proof

const TreeDepth = 256

type MerkleProof struct {
	Version  uint64
	Exists   bool
	LeafHash [32]byte
	Siblings [TreeDepth][32]byte
}
