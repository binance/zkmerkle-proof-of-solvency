package circuit

import (
	"github.com/binance/zkmerkle-proof-of-solvency/src/utils"
	"github.com/consensys/gnark/frontend"
)

type (
	Variable = frontend.Variable
	API      = frontend.API
)

// Consider using variable or constant
type TierRatio struct {
	BoundaryValue    Variable
	Ratio            Variable
	PrecomputedValue Variable
}

type CexAssetInfo struct {
	TotalEquity Variable
	TotalDebt   Variable
	BasePrice   Variable

	LoanCollateral            Variable
	MarginCollateral          Variable
	PortfolioMarginCollateral Variable

	LoanRatios            []TierRatio
	MarginRatios          []TierRatio
	PortfolioMarginRatios []TierRatio
}

type UserAssetInfo struct {
	AssetIndex Variable
	// The index means the position of tier ratios where the boundary value is larger than the collateral.
	LoanCollateralIndex Variable
	// If the flag is 1, the boundary value of last tier ratio is less than the collateral.
	LoanCollateralFlag Variable

	MarginCollateralIndex Variable
	MarginCollateralFlag  Variable

	PortfolioMarginCollateralIndex Variable
	PortfolioMarginCollateralFlag  Variable
}

type UserAssetMeta struct {
	Equity                    Variable
	Debt                      Variable
	LoanCollateral            Variable
	MarginCollateral          Variable
	PortfolioMarginCollateral Variable
}

type CreateUserOperation struct {
	BeforeAccountTreeRoot Variable
	AfterAccountTreeRoot  Variable
	Assets                []UserAssetInfo
	AssetsForUpdateCex    []UserAssetMeta
	AccountIndex          Variable
	AccountIdHash         Variable
	AccountProof          [utils.AccountTreeDepth]Variable
}
