package circuit

import "math/big"

var (
	//  is poseidon hash(empty account info)
	EmptyAccountLeafNodeHash, _ = new(big.Int).SetString("1cc3ed1b0e66b98f30582ec8a02dd32bd938edc613e7616076f5c03d3c04f02f", 16)
)
