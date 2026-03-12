# ZK Merkle Proof of Solvency (PoR)

## Project Overview

Binance's zero-knowledge Proof of Reserves system. Uses Groth16 zk-SNARKs (gnark over BN254) to prove that a CEX holds sufficient assets to cover all user balances without revealing individual user data.

**Module**: `github.com/binance/zkmerkle-proof-of-solvency`

**Go version**: 1.22 (toolchain 1.23.1)


## Architecture & Data Flow

```
                      ┌──> MySQL (witness table) ──> prover ──> MySQL (proof table) ──> verifier
User CSV files ──> witness
                      └──> MySQL (userproof table) ──> verifier -user
```

### Services (5 executables)

| Service | Entry Point | Description |
|---------|------------|-------------|
| **keygen** | `src/keygen/main.go` | Generates Groth16 pk/vk/r1cs files per circuit tier |
| **witness** | `src/witness/main.go` | Parses user data, builds Merkle tree, generates batch witnesses + user proofs |
| **dbtool** | `src/dbtool/main.go` | Database management CLI (push tasks to Redis, query data, cleanup) |
| **prover** | `src/prover/main.go` | Fetches witness from Redis queue, generates Groth16 proofs |
| **verifier** | `src/verifier/main.go` | Verifies batch proofs (CSV) or individual user proofs |

## Key Constants

```go
AccountTreeDepth = 28                    // supports ~268M accounts
AssetCounts      = 500                   // total asset types tracked
TierCount        = 12                    // collateral ratio tiers

// Multi-tier circuit: asset count -> users per batch
BatchCreateUserOpsCountsTiers = map[int]int{
    500: 200,   // users with many assets
    50:  1380,  // users with few assets
}
AssetCountsTiers = [50, 500]             // sorted tier keys
```

## Directory Structure

```
circuit/                           # ZK circuit definitions (gnark)
  batch_create_user_circuit.go     #   main circuit: BatchCreateUserCircuit
  types.go                         #   circuit-side types (Variable-based)
  utils.go                         #   verifyMerkleProof, collateral helpers
src/
  utils/
    types.go                       #   core domain types (AccountInfo, CexAssetInfo, etc.)
    constants.go                   #   AccountTreeDepth, AssetCounts, tier configs
    utils.go                       #   CSV parsing, hashing, serialization
    account_tree.go                #   NewAccountTree(), VerifyMerkleProof()
    merkletree/merkletree.go       #   FixedDepthMerkleTree (in-memory, two-phase)
  witness/
    main.go                        #   witness entry point
    config/config.go               #   witness config struct
    witness/witness.go             #   batch witness generation
    witness/userproof.go           #   per-user Merkle proof generation
  prover/
    main.go                        #   prover entry point
    config/config.go               #   prover config struct
    prover/prover.go               #   proof generation logic
    prover/proof_model.go          #   Proof DB model
  verifier/
    main.go                        #   verifier entry point (batch + user modes)
    config/config.go               #   verifier + user config structs
  keygen/main.go                   #   key generation entry point
  dbtool/main.go                   #   database management CLI
```

## Circuit Design

**`BatchCreateUserCircuit`** — single public input: `BatchCommitment`

```
BatchCommitment = Poseidon(AccountTreeRoot, BeforeCEXAssetsCommitment, AfterCEXAssetsCommitment, MinAccountIndex, MaxAccountIndex)
```

Per-user constraints:
- Verify account Merkle proof against `AccountTreeRoot` (fixed, read-only tree)
- AccountIndex must increment by exactly 1 between consecutive users in a batch
- First user's AccountIndex == `MinAccountIndex`, last user's == `MaxAccountIndex`
- Asset equity/debt/collateral consistency checks
- Collateral ratio tier verification via log-derivative lookup tables
- Random linear combination check for asset completeness

Cross-batch verification (verifier side):
- All batches share the same `AccountTreeRoot`
- CEX asset commitments chain: batch N's `AfterCEXAssetsCommitment` == batch N+1's `BeforeCEXAssetsCommitment`
- Account indices are contiguous: batch N's `MaxAccountIndex` + 1 == batch N+1's `MinAccountIndex`

## Merkle Tree

**`FixedDepthMerkleTree`** — fully in-memory, fixed depth 28, Poseidon hash

Two-phase usage:
1. `Set(key, value)` — store leaves (parallel-safe for distinct keys)
2. `Build()` — compute internal nodes bottom-up in parallel
3. `GetProof(key)` / `Root()` — read-only queries

Empty leaf: `NilAccountHash = Poseidon(0,0,0,0,0)`

**How PoR usage patterns shape the Merkle tree implementation**: PoR must prove hundreds of millions of accounts (tree depth 28, supporting up to ~268M accounts). These accounts are densely packed in the tree — leaf indices start at 0 and increment consecutively with no gaps. This means all valid accounts occupy a contiguous region on the left side of the tree, while the vast majority of leaves on the right are empty (`NilAccountHash`). This usage pattern directly influences the `FixedDepthMerkleTree` implementation strategy: storage is only allocated for accounts that actually exist, and empty leaves along with their empty subtrees are handled via precomputed default hashes, yielding significant savings in both memory and computation.

## Key Dependencies

- **gnark** v0.10 (via `bnb-chain/gnark` fork) — ZK-SNARK framework
- **gnark-crypto** v0.14 (via `bnb-chain/gnark-crypto` fork) — Poseidon hash, BN254 curve
- **gorm** + MySQL — batch witness / proof / userproof storage
- **go-redis** — task queue for prover job distribution
- **s2 (klauspost/compress)** — witness data compression

## Common Commands

```bash
# Build all
go build ./...

# Run circuit unit test (fast, 4 users per batch)
go test ./circuit/ -run '^TestBatchCreateUserCircuit$' -v -timeout 300s

# Run full key setup test (slow)
go test ./circuit/ -run '^TestBatchCreateUserCircuitFromKeySetup$' -v -timeout 1h

# Run all utils tests
go test ./src/utils/ -v
```

## Development Notes

- Circuit changes require updating: `circuit/` -> `src/witness/witness/` -> `src/prover/prover/` -> `src/verifier/main.go` -> `circuit/*_test.go`
- Test uses 4 users per batch (not real 200/1380) for speed
- `BatchCommitment` is the only public input to the circuit; all verification derives from it
- The `replace` directives in `go.mod` point gnark/gnark-crypto to bnb-chain forks
