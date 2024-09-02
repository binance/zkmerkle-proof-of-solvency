package utils

import "math/big"

type TierRatio struct {
	BoundaryValue    *big.Int
	Ratio            uint8
	PrecomputedValue *big.Int
}

type CexAssetInfo struct {
	TotalEquity               uint64
	TotalDebt                 uint64
	BasePrice                 uint64
	Symbol                    string
	Index                     uint32
	LoanCollateral            uint64
	MarginCollateral          uint64
	PortfolioMarginCollateral uint64
	LoanRatios                [TierCount]TierRatio
	MarginRatios              [TierCount]TierRatio
	PortfolioMarginRatios     [TierCount]TierRatio
}

type AccountAsset struct {
	Index           uint16
	Equity          uint64
	Debt            uint64
	Loan            uint64
	Margin          uint64
	PortfolioMargin uint64
}

type AccountInfo struct {
	AccountIndex    uint32
	AccountId       []byte
	TotalEquity     *big.Int
	TotalDebt       *big.Int
	TotalCollateral *big.Int
	Assets          []AccountAsset
}

type CreateUserOperation struct {
	BeforeAccountTreeRoot []byte
	AfterAccountTreeRoot  []byte
	Assets                []AccountAsset
	AccountIndex          uint32
	AccountIdHash         []byte
	AccountProof          [AccountTreeDepth][]byte
}

type BatchCreateUserWitness struct {
	BatchCommitment           []byte
	BeforeAccountTreeRoot     []byte
	AfterAccountTreeRoot      []byte
	BeforeCEXAssetsCommitment []byte
	AfterCEXAssetsCommitment  []byte

	BeforeCexAssets []CexAssetInfo
	CreateUserOps   []CreateUserOperation
}
