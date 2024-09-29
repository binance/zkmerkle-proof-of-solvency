package circuit

import "math/big"

var (
	//  is poseidon hash(empty account info)
	EmptyAccountLeafNodeHash, _ = new(big.Int).SetString("0f870d7404597dad9eca7c50a6f0af812ab7cd6a11d5c464d4031a3272377b95", 16)
)
