# zkmerkle-proof-of-solvency
## Circuit Design

See the [technical blog](./docs/updated_proof_of_solvency_to_mitigate_dummy_user_attack.md) for more details about background and circuit design

### Circuit Performance Benchmark

#### Constraint Count

The base and per-user constraint counts depend on `AssetCounts` (the total
number of asset types the circuit tracks, defined in `src/utils/constants.go`),
because the base cost and each user's per-asset bookkeeping both scale with it.
The numbers below can be reproduced with the `TestEstimateUserCapacityPerTier`
test in `circuit/batch_create_user_circuit_test.go`, which compiles the circuit
for 1..4 users per batch and linearly fits `constraints(n) = base + perUser·n`.

##### When `AssetCounts = 500`

| Component | Constraints |
|-----------|------------|
| Base (shared across all users in a batch) | ~6,630,000 |
| Per user — 50-asset tier | ~42,300 |
| Per user — 500-asset tier | ~281,200 |

##### When `AssetCounts = 700`

| Component | Constraints |
|-----------|------------|
| Base (shared across all users in a batch) | ~9,252,280 |
| Per user — 50-asset tier | ~45,293 |
| Per user — 500-asset tier | ~284,098 |
| Per user — 700-asset tier | ~390,160 |

#### How `BatchCreateUserOpsCountsTiers` Is Determined

The maximum constraint count supported by the underlying ZK algorithm (Groth16 over BN254 via gnark) is 2^28. We target 2^26 constraints per batch to leave a safety margin. The per-tier batch sizes are derived as follows:

##### When `AssetCounts = 500`

- **500-asset tier**: (2^26 − 6,630,000) / 281,200 ≈ 215 → rounded down to **200**
- **50-asset tier**: (2^26 − 6,575,000) / 42,300 ≈ 1430 → rounded down to **1380**

##### When `AssetCounts = 700`

Raising `AssetCounts` to 700 increases the fixed base cost to ~9,252,280 and adds a third tier for users holding 501–700 asset types. The theoretical maximum users per batch (from `TestEstimateUserCapacityPerTier` under a 2^26 budget) and the configured `BatchCreateUserOpsCountsTiers` values are:

- **700-asset tier**: (2^26 − 9,252,280) / 390,160 ≈ 148 → rounded down to **128**
- **500-asset tier**: (2^26 − 9,252,280) / 284,098 ≈ 203 → rounded down to **192**
- **50-asset tier**: (2^26 − 9,252,280) / 45,293 ≈ 1277 → rounded down to **1216**

These are the values currently set in `BatchCreateUserOpsCountsTiers` (`700: 128, 500: 192, 50: 1216`). The rounded-down batch sizes leave extra headroom below the theoretical maximum.


## How to run

### Run third-party services
This project needs following third party services:
- mysql: used to store `witness`, `userproof`, `proof` table;
- redis: used as task queue for distributing proof generation tasks to provers;

We can use docker to run these services:

```shell
docker run -d --name zkpos-redis -p 6379:6379 redis

docker run -d --name zkpos-mysql -p 3306:3306  -v /server/docker_data/mysql_data:/var/lib/mysql -e MYSQL_USER=zkpos -e MYSQL_PASSWORD=zkpos@123 -e MYSQL_DATABASE=zkpos  -e MYSQL_ROOT_PASSWORD=zkpos@123 mysql
```

where `/server/docker_data/` is directory in the host machine which is used to persist mysql docker data.


### Generate zk keys

The `keygen` service is for generating zk related keys which are used to generate and verify zk proof. The updated PoR solution now supports multi-tier circuits based on the counts of asset types a user owns. The `BatchCreateUserOpsCountsTiers` constant in the utils package represents the multi-tier circuit configuration that defines how many users can be created in one batch for each specific tier.

Run the following commands to start `keygen` service:
```
// make sure the BatchCreateUserOpsCounts in utils/constants.go is expected

cd src/keygen; go run main.go
```

After `keygen` service finishes running, there will be several key files generated in the current directory, like the following:
```shell
-rw-r--r--. 1 root root  524 Aug 19 09:46 zkpor500_200.vk
-rw-r--r--. 1 root root  12G Aug 19 09:46 zkpor500_200.pk
-rw-r--r--. 1 root root 7.5G Aug 19 09:47 zkpor500_200.r1cs
-rw-r--r--. 1 root root  524 Aug 19 10:38 zkpor50_1380.vk
-rw-r--r--. 1 root root  12G Aug 19 10:38 zkpor50_1380.pk
-rw-r--r--. 1 root root  12G Aug 19 10:39 zkpor50_1380.r1cs
```

### Generate witness

The `witness` service is used to generate witness for `prover` service and user merkle proofs for user verification. The account tree is built entirely in memory using a fixed-depth Merkle tree.

`witness/config/config.json` is the config file `witness` service use. The sample file is as follows:
```json
{
  "MysqlDataSource" : "zkpos:zkpos@123@tcp(127.0.0.1:3306)/zkpos?parseTime=true",
  "UserDataFile": "/server/data/20230118",
  "DbSuffix": "0"
}
```

Where

- `MysqlDataSource`: this is the mysql config;
- `UserDataFile`: the directory which contains all users balance sheet files;
- `DbSuffix`: this suffix will be appended to the ending of table name, such as `proof0`, `witness0` table;


Run the following command to start `witness` service:
```shell
cd witness; go run main.go
```

The `witness` service supports recovery from unexpected crash. After `witness` service finish running, we can see `witness` from `witness` table and user proofs from `userproof` table.

One witness batch contains 1380 users whose assets number is less or equal than 50, and 200 users whose assets number is larger than 50.

### Push Task to Redis
The `db_tool` cli provide a subcommand called `push_task_to_redis` which can be used for push proof generating tasks to redis after all the witnesses data are generated. The provers will fetch the proof-generating tasks from redis, update the witness data status into `received`, then generate the proof, and update the witness data status into `finished`.

### Generate zk proof

The `prover` service is used to generate zk proof and supports running in parallel. It reads witness from `witness` table generated by `witness` service.

`prover/config/config.json` is the config file `prover` service uses. The sample file is as follows:
```json
{
  "MysqlDataSource" : "zkpos:zkpos@123@tcp(127.0.0.1:3306)/zkpos?parseTime=true",
  "DbSuffix": "0",
  "Redis": {
    "Host": "127.0.0.1:6379",
  },
  "ZkKeyName": ["/server/zkmerkle-proof-of-solvency/src/keygen/zkpor50_1380", "/server/zkmerkle-proof-of-solvency/src/keygen/zkpor500_200"],
  "AssetsCountTiers": [50, 500]
}
```

Where

- `MysqlDataSource`: this is the mysql config;
- `DbSuffix`: this suffix will be appended to the ending of table name, such as `proof0`, `witness0` table;
- `Redis`:
  - `Host`: `redis` service listen addr;
  - `Type`: only support `node` type
- `ZkKeyName`: the list of key names generated by `keygen` service
- `AssetsCountTiers`: The list of asset count tiers, each corresponding to a key name in `ZkKeyName`

Run the following command to start `prover` service:
```shell
cd prover; go run main.go
```

To run `prover` service in parallel, just repeat executing above commands.

**Note: After all prover service finishes running, We should use `go run main.go -rerun` command to regenerate proof for unfinished batch**

After the whole `prover` service finished, we can see batch zk proof in `proof` table.

### Verifier

The `verifier` service is used to verify batch proof and single user proof.

#### Verify batch proof
The service use `config.json` as its config file, and the sample config is as follows:
```json
{
  "ProofTable": "config/proof.csv",
  "ZkKeyName": ["config/zkpor50_1380", "config/zkpor500_200"],
  "AssetsCountTiers": [50, 500],
  "CexAssetsInfo": [{"TotalEquity":219971568487,"TotalDebt":9789219,"BasePrice":24620000000},{"TotalEquity":8664493444,"TotalDebt":122580,"BasePrice":1682628000000},{"TotalEquity":67463930749983,"TotalDebt":16127314913,"BasePrice":100000000},{"TotalEquity":68358645578,"TotalDebt":130187,"BasePrice":121377000000},{"TotalEquity":590353015932,"TotalDebt":0,"BasePrice":598900000},{"TotalEquity":255845425858,"TotalDebt":13839361,"BasePrice":6541000000},{"TotalEquity":0,"TotalDebt":0,"BasePrice":99991478},{"TotalEquity":267958065914051,"TotalDebt":501899265949,"BasePrice":100000000},{"TotalEquity":124934670143615,"TotalDebt":1422964747,"BasePrice":34500000}]
}
```
Where
- `ProofTable`: this is proof csv file which can be exported by `proof` table;
- `ZkKeyName`: the key name generated by `keygen` service;
- `AssetsCountTiers`: The list of asset count tiers, each corresponding to a key name in `ZkKeyName`;
- `CexAssetsInfo`: this is published by CEX, it represents CEX's liability;

You can get `CexAssetsInfo` using `dbtool` command after `witness` service run finished. Run the following command to verify batch proof:
```shell
cd verifier; go run main.go
```

#### Verify user proof
The service use `user_config.json` as its config file, and the sample config is as follows:
```json
{"AccountIndex":9,"AccountIdHash":"000000000000000000000000000000000000000000000000000000000000006d","TotalEquity":107595993240612342000000,"TotalDebt":4812541145779934000000,"TotalCollateral":4861152676874104600000,"Assets":[{"Index":0,"Equity":14571647457,"Debt":184812783,"Loan":7285823729,"Margin":3642911864,"PortfolioMargin":1821455932},{"Index":1,"Equity":25424316291,"Debt":3323064077,"Loan":12712158145,"Margin":6356079073,"PortfolioMargin":3178039536},{"Index":2,"Equity":57834282404,"Debt":19716095367,"Loan":28917141202,"Margin":14458570601,"PortfolioMargin":7229285300},{"Index":3,"Equity":25100,"Debt":669524367015,"Loan":12550,"Margin":6275,"PortfolioMargin":3138}],"Root":"2cc3d21f8cb334666db54fd08694f0016d7615c145903e636f13e8a3bfc8c098","Proof":["D4cNdARZfa2eynxQpvCvgSq3zWoR1cRk1AMaMnI3e5U=","FrDt3GyLckWS8K8jDqiLBwd+uYhK1slj4BWJyRly/xY=","IvLcujfEoCJeOpCd0DqEWxJYioF/ky8BqYo6CfVn0aM=","Bfd935sQkaFG0u6keaAr3kwfZtxT7aBLxpjBv87mxEI=","GOzvFDJcnCQGIlpR/i/XPSMf1Ztr+Bi8flHwQ/2OtvU=","KkclBagJApH2Yp8M+mKrIFnU7LjQt28EMiPMpH6dQmk=","FWhhjzWzScHRDmMaOvmz1y2H8seNEpVt+5zxSpkbL5g=","F//tYykHrOwPc2eL2n9loqE4cCPmS7oT82nuWdKVS5I=","BQUW/GSyx7d++VLtqlJgB3UgS3td00kBZwx+gYqdhkI=","CX3CsFsCDe/uLqgF32dN6opeP6zsby5OtyH65WlTOFo=","GwKU6iMGHDT/cWXO1GQ75FUTobiz85RXvN1f35KcQuI=","Ksn5vEKEYp16F+vvblsaB5+F86M20kig9VKjDuzraLw=","HY1KU47xTzOWvz2DJzBwO1vO+wZ7tbbmBfhWuD09liQ=","Dr0CiB0xe+l+KsCQMr5fBEBh5yDU3R2w7qEGmHzrUCw=","HiyLbCOIJ7J4eQ2sgp+HWtYFypWwv85BEflMIfcEu1g=","Ck4RLJEEqw4f2W4BkepiwqtuH2vP4o/pJ0Y2daob6VU=","B9qUG9+LeC2odoe12fBnlkHJAJ5AeGx1TkIlMqzAWIY=","JmYsJrMBXScyekhIX375R5JlA14Kf0YuR4d33zWBWG8=","Chis1F143VuP/E97TnSwljQE2sjii8seHGCA/UrZw1c=","BSyAGaUwsJSRkIr9W3sRedR1MH4mBrReIhnKlQ+GqAU=","F/C7Bs7czyih/hhRynRGMJR7+VnFtNF4NvEDtR9KSjU=","BS65qpc3OdxH7f2us+qt5YS1J13/OIFFSFJ8XdYcf7s=","A8nLyvB+T0XBOVBpUAPD5G3FIzpKEst5FyN1/E8x6X4=","IBvTojZBKA9Hoe8T2/wmWMFXclscRA01TT7U/P9Hdkw=","G2CxLbhRmc1ed2P1r+m+0txINsdfWyQXlb0Hz8/RVSo=","K/PKFaI2uejoWJTwttXwdobZrw06oEK6ncCN+PLhmCI=","A0kdSxKzRgbWZ0/R2yI9E+xpxx4uZhElwln19XPEhyo=","FbMFF3x1BrXP88vc65fB2tp+M1lMaSGuqOL04xPquaY="]}
```

Where

- `AccountIndex`: account index used to verify;
- `AccountIdHash`: account hash id which contains user info
- `Root`: account tree root published by cex;
- `Assets`: all user assets info;
- `Proof`: user merkle proof which uses `base64` encoding;
- `TotalEquity`: user total equity which is calculated by all the assets equity multipy its corresponding price
- `TotalDebt`: user total debt which is calculated by all the assets debt multipy its corresponding price
- `Loan/Margin/PortfolioMargin`: user collateral value

Run the following command to verify single user proof:
```shell
cd verifier; go run main.go -user
```

### dbtool command

Run the following command to delete mysql and redis data:
```shell
cd src/dbtool; go run main.go -delete_all
```

Run the following command to get cex assets info in json format:
```shell
cd src/dbtool; go run main.go -query_cex_assets
```

Run the following command to query user config which is used in verifier:
```shell
cd src/dbtool; go run main.go -query_account_data 9
```

Run the following command to query witness data which is the input of circuit:
```shell
cd src/dbtool; go run main.go -query_witness_data 9
```
