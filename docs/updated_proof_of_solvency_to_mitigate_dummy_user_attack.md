# Updated Proof of solvency for mitigating Dummy user attack

## Background

Previous Proof of solvency design: [https://gusty-radon-13b.notion.site/Proof-of-solvency-61414c3f7c1e46c5baec32b9491b2b3d](https://www.notion.so/Proof-of-solvency-61414c3f7c1e46c5baec32b9491b2b3d?pvs=21).

Enrico Bottazzi, research at the Ethereum Foundation, has reported an alarming phenomenon dubbed the "Dummy User Attack." This attack unfolds as follows: User of this framework has the ability to introduce dummy users whose equity in relatively low-market-cap assets surpass their debts in high-market-cap ones. Such actions are allowed by the current proof-of-solvency design. This manipulation can lead to a precarious scenario wherein User of this framework diminishes the on-chain reserve of high-market-cap assets while inflating that of low-market-cap ones. Consequently, when users seek to withdraw their high-market-cap assets, User of this framework is compelled to liquidate the low-market-cap assets. However, this liquidation may prove unfeasible due to inadequate market liquidity for the low-market-cap assets at times, exacerbating the potential risks for users.

## Updated Proof of solvency protocol

To mitigate the “Dummy user attack”, The updated proof of solvency design introduces a third field, labeled "collateral" within the token configuration for each user. This field denotes the quantity of tokens utilized as collateral for borrowing other assets. Simultaneously, within the global token configuration, fields titled `collateral_ratio_tiers` and `collateral_asset_amount` are introduced, delineating the extent of collateral haircut for the asset involved and the amount of the collateral asset. For further elucidation, refer to the respective [page](https://www.binance.com/en/vip-loan). Under the loan business logic of Binance, this design effectively incorporates three distinct collateral fields catering to various loan businesses: VIP LOAN, COLLATERAL MARGIN, and COLLATERAL PORTFOLIO MARGIN. Consequently, the global token configuration encompasses three categories of collateral ratio tiers and amounts to accommodate these distinctions.

Based on above description, Let’s see a concrete example:

Suppose there are 2 assets (ETH, USDT) and 3 users, and use USDT as the base denominated asset. Assume following user behaviors:

- Alice deposit 10000 USDT to CEX, then use 10000 USDT as VIP LOAN collateral to borrow 2 ETH, and then use 1 ETH as VIP LOAN collateral to borrow 1000 USDT; and swap 1 ETH with Bob's 2000 USDT
- Bob deposit 10 ETH and 10000 USDT to CEX; swap 2000 USDT with Alice's 1 ETH
- Carl deposit 10 ETH to CEX, then use 1 ETH as COLLATERAL MARGIN collateral to borrow 1000 USDT

The user's balance sheet is as following:

|  | ETH (price:2000 USDT) | ETH | ETH  | ETH | ETH | USDT (price: 1 USDT) | USDT | USDT | USDT | USDT | Total Net Balance (USDT) |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
|  | Equity | Debt | COLLATERAL VIP LOAN | COLLATERAL MARGIN | COLLATERAL PORTFOLIO MARGIN | Equity | Debt | COLLATERAL VIP LOAN | COLLATERAL MARGIN | COLLATERAL PORTFOLIO MARGIN |  |
| Alice | 1 | 2 | 1 | 0 | 0 | 13000 | 1000 | 10000 | 0 | 0 | 10000 |
| Bob | 11 | 0 | 0 | 0 | 0 | 8000 | 0 | 0 | 0 | 0 | 30000 |
| Carl | 10 | 0 | 0 | 1 | 0 | 1000 | 1000 | 0 | 0 | 0 | 20000 |

The token’s configuration is as following:

| Token Symbol | Total Equity | Total Debt | Price | COLLATERAL VIP LOAN amount | COLLATERAL MARGIN amount | COLLATERAL PORTFOLIO MARGIN amount | COLLATERAL VIP LOAN ratio tiers | COLLATERAL MARGIN ratio tiers | COLLATERAL PORTFOLIO MARGIN ratio tiers |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| ETH | 22 | 2 | 2000 | 1 | 1 | 0 | [0-1000: 100%, 1000-2000: 90%, …, > 1000000: 0%] | […] | […] |
| USDT | 22000 | 2000 | 1 | 10000 | 0 | 0 | [0-10000: 100%, 10000-20000: 90%, …, > 1000000: 0%] | […] | […] |

*Note: The essence of ratio tiers lies in their values, which are based on USDT as the denominated asset. Consider ETH as an example: "0-1000": 100% signifies that you can utilize 100% of the ETH's USDT value as collateral when the USDT value of ETH is less than or equal to 1000. "1000-2000": 90% indicates that you can use 100% of the ETH value within the 0-1000 range as collateral and 90% of the value within the "1000-2000" range for borrowing purposes.*

The assets CEX holds equal to the summation of every user net asset balance (Equity-Debt). So the CEX needs to hold 20 ETH and 20000 USDT at least.

In proof of solvency, the following properties are guaranteed:

- Each user's collaterals are enough to cover the debts according to the collateral ratio tiers: $\sum_{i \in tokens}{Debt_i} \leq \sum_{ij, i \in tokens, j \in collaterals}{CalcActualAssetValueByRatioTiers(Collateral_{ij})}$;
    - Consider Alice as the example: Her debt is calculated as following: 2(ETH debt)*2000(ETH usdt price) + 1000(USDT debt)*1 = 5000; Her collateral is calculated as following: 1(ETH VIP LOAN collateral)*2000 + 10000(USDT VIP LOAN collateral)*1 = 12000; 12000 > 5000, So Alice can be added into the por proof generation process.
- For each user, Each token’s equity should be equal or bigger than the sum of collaterals: $\sum_{i \in collaterals}{Collateral_i} \leq Equity$;
    - Consider Alice as the example: For ETH token, the collateral amouts is calculated as following: 1+0+0 ≤ 1; For USDT token, 10000+0+0 < 13000.
- Each asset of user is a part of total net asset declared by CEX:
    - For alice, -1 eth is a part of total net eth, and 12000 usdt is a part of total net usdt ;
    - For Bob, 11 eth is a part of total net eth, and 8000 usdt is a part of total net usdt;
    - For Carl, 10 eth is a part of total net eth, and 0 usdt is a part of total net usdt
- the total net asset declared by CEX equals to the summation of every user net asset balance;
- the total collateral asset declared by CEX equals to the summation of every user collateral asset amount

How does the introduced collateral design mitigate the dummy user attack? The global configuration includes ratio tiers and collateral asset amount for each asset. The collateral ratio of low-market-cap assets is low. If User of this framework were to attempt a dummy user attack, it would need to add more low-market-cap assets to replace high-market-cap assets. Users can detect this attack by comparing the ratio between the total collateral asset of low-market-cap assets and the total debt of high-market-cap assets. Additionally, users can verify whether the on-chain reserves of low-market-cap assets can cover the claimed net balance in the asset configuration. If not, it further demonstrates fraudulent behavior by User of this framework.