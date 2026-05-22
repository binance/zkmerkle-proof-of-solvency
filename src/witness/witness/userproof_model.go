package witness

import (
	"fmt"
	"math/big"

	"github.com/binance/zkmerkle-proof-of-solvency/src/utils"
	"gorm.io/gorm"
)

const UserProofTableNamePrefix = "userproof"

type (
	UserProofModel interface {
		CreateUserProofTable() error
		DropUserProofTable() error
		CreateUserProofs(rows []UserProof) error
		// CreateAccountIdIndex adds a non-unique index on account_id after all
		// rows have been inserted. AccountId is a Poseidon hash of user data,
		// so duplicates are practically impossible; a uniqueness constraint is
		// unnecessary. Building the index in one pass (sort + bulk-load) is far
		// cheaper than maintaining it during 100M+ row inserts.
		CreateAccountIdIndex() error
		GetUserProofByIndex(id uint32) (*UserProof, error)
		GetUserProofById(id string) (*UserProof, error)
		GetLatestAccountIndex() (uint32, error)
		GetUserCounts() (int, error)
	}

	defaultUserProofModel struct {
		table string
		DB    *gorm.DB
	}

	// UserProof stores a per-user Merkle inclusion proof.
	// account_id has no unique index because the table holds ~100M rows and
	// maintaining a unique B-tree on random hash strings causes severe random
	// I/O once the index exceeds the InnoDB buffer pool, degrading bulk-insert
	// throughput. The index is created after all rows are written instead
	// (see UserProofModel.CreateAccountIdIndex).
	UserProof struct {
		AccountIndex    uint32 `gorm:"index:idx_int,unique"`
		AccountId       string `gorm:"type:varchar(64)"`
		AccountLeafHash string
		TotalEquity     string
		TotalDebt       string
		TotalCollateral string
		Assets          string
		Proof           string
		Config          string
	}

	UserConfig struct {
		AccountIndex    uint32
		AccountIdHash   string
		TotalEquity     *big.Int
		TotalDebt       *big.Int
		TotalCollateral *big.Int
		Assets          []utils.AccountAsset
		Root            string
		Proof           [][]byte
	}
)

func (m *defaultUserProofModel) TableName() string {
	return m.table
}

func NewUserProofModel(db *gorm.DB, suffix string) UserProofModel {
	return &defaultUserProofModel{
		table: UserProofTableNamePrefix + suffix,
		DB:    db,
	}
}

func (m *defaultUserProofModel) CreateUserProofTable() error {
	return m.DB.Table(m.table).AutoMigrate(UserProof{})
}

func (m *defaultUserProofModel) DropUserProofTable() error {
	return m.DB.Migrator().DropTable(m.table)
}

func (m *defaultUserProofModel) CreateAccountIdIndex() error {
	sql := fmt.Sprintf("ALTER TABLE `%s` ADD INDEX `idx_str` (`account_id`)", m.table)
	return m.DB.Exec(sql).Error
}

func (m *defaultUserProofModel) CreateUserProofs(rows []UserProof) error {
	dbTx := m.DB.Table(m.table).Create(rows)
	if dbTx.Error != nil {
		return dbTx.Error
	}
	return nil
}

func (m *defaultUserProofModel) GetUserProofByIndex(id uint32) (userproof *UserProof, err error) {
	userproof = &UserProof{}
	dbTx := m.DB.Clauses(utils.MaxExecutionTimeHint).Table(m.table).Where("account_index = ?", id).Find(userproof)
	if dbTx.Error != nil {
		return nil, utils.ConvertMysqlErrToDbErr(dbTx.Error)
	} else if dbTx.RowsAffected == 0 {
		return nil, utils.DbErrNotFound
	}
	return userproof, nil
}

func (m *defaultUserProofModel) GetUserProofById(id string) (userproof *UserProof, err error) {
	userproof = &UserProof{}
	dbTx := m.DB.Clauses(utils.MaxExecutionTimeHint).Table(m.table).Where("account_id = ?", id).Find(userproof)
	if dbTx.Error != nil {
		return nil, utils.ConvertMysqlErrToDbErr(dbTx.Error)
	} else if dbTx.RowsAffected == 0 {
		return nil, utils.DbErrNotFound
	}
	return userproof, nil
}

func (m *defaultUserProofModel) GetLatestAccountIndex() (uint32, error) {
	var row *UserProof
	dbTx := m.DB.Clauses(utils.MaxExecutionTimeHint).Table(m.table).Order("account_index desc").Limit(1).Find(&row)
	if dbTx.Error != nil {
		return 0, utils.ConvertMysqlErrToDbErr(dbTx.Error)
	} else if dbTx.RowsAffected == 0 {
		return 0, utils.DbErrNotFound
	}
	return row.AccountIndex, nil
}

func (m *defaultUserProofModel) GetUserCounts() (int, error) {
	var count int64 = 0
	dbTx := m.DB.Clauses(utils.MaxExecutionTimeHint).Table(m.table).Count(&count)
	if dbTx.Error != nil {
		return 0, utils.ConvertMysqlErrToDbErr(dbTx.Error)
	}
	return int(count), nil
}
