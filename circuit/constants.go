package circuit

import "math/big"

var (
	//  is poseidon hash(empty account info)
	EmptyAccountLeafNodeHash, _ = new(big.Int).SetString("163607ac0eaf42c44a36448da92f3a29f1943659df740f2490d47ddcd40ee672", 16)
)
