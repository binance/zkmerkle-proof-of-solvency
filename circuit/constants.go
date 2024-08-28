package circuit

import "math/big"

var (
	//  is poseidon hash(empty account info)
	EmptyAccountLeafNodeHash, _ = new(big.Int).SetString("0450b4105d6f44db0c827ac0ead6cd4947d878ee70af627710faa1d8cf6c45f5", 16)
)
