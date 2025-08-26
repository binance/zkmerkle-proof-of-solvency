package witness

import (
	"time"

	"github.com/binance/zkmerkle-proof-of-solvency/src/utils"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	StatusPublished = iota
	StatusReceived
	StatusFinished
)

const (
	TableNamePrefix = `witness`
)

type (
	WitnessModel interface {
		CreateBatchWitnessTable() error
		DropBatchWitnessTable() error
		GetLatestBatchWitnessHeight() (height int64, err error)
		GetBatchWitnessByHeight(height int64) (witness *BatchWitness, err error)
		UpdateBatchWitnessStatus(witness *BatchWitness, status int64) error
		GetLatestBatchWitness() (witness *BatchWitness, err error)
		GetLatestBatchWitnessByStatus(status int64) (witness *BatchWitness, err error)
		GetAllBatchHeightsByStatus(status int64, limit int, offset int) (witnessHeights []int64, err error)
		GetAndUpdateBatchesWitnessByStatus(beforeStatus, afterStatus int64, count int32) (witness [](*BatchWitness), err error)
		GetAndUpdateBatchesWitnessByHeight(height int, beforeStatus, afterStatus int64) (witness [](*BatchWitness), err error)
		CreateBatchWitness(witness []BatchWitness) error
		GetRowCounts() (count []int64, err error)
	}

	defaultWitnessModel struct {
		table string
		DB    *gorm.DB
	}

	BatchWitness struct {
		gorm.Model
		Height      int64 `gorm:"index:idx_height,unique"`
		WitnessData string
		Status      int64 `gorm:"index"`
	}
)

func NewWitnessModel(db *gorm.DB, suffix string) WitnessModel {
	return &defaultWitnessModel{
		table: TableNamePrefix + suffix,
		DB:    db,
	}
}

func (m *defaultWitnessModel) TableName() string {
	return m.table
}

func (m *defaultWitnessModel) CreateBatchWitnessTable() error {
	return m.DB.Table(m.table).AutoMigrate(BatchWitness{})
}

func (m *defaultWitnessModel) DropBatchWitnessTable() error {
	return m.DB.Migrator().DropTable(m.table)
}

func (m *defaultWitnessModel) GetLatestBatchWitnessHeight() (batchNumber int64, err error) {
	var height int64
	dbTx := m.DB.Clauses(utils.MaxExecutionTimeHint).Table(m.table).Select("height").Order("height desc").Limit(1).Find(&height)
	if dbTx.Error != nil {
		return 0, utils.ConvertMysqlErrToDbErr(dbTx.Error)
	} else if dbTx.RowsAffected == 0 {
		return 0, utils.DbErrNotFound
	}
	return height, nil
}

func (m *defaultWitnessModel) GetLatestBatchWitness() (witness *BatchWitness, err error) {
	var height int64
	dbTx := m.DB.Clauses(utils.MaxExecutionTimeHint).Table(m.table).Debug().Select("height").Order("height desc").Limit(1).Find(&height)
	if dbTx.Error != nil {
		return nil, utils.ConvertMysqlErrToDbErr(dbTx.Error)
	} else if dbTx.RowsAffected == 0 {
		return nil, utils.DbErrNotFound
	}

	return m.GetBatchWitnessByHeight(height)
}

func (m *defaultWitnessModel) GetLatestBatchWitnessByStatus(status int64) (witness *BatchWitness, err error) {
	dbTx := m.DB.Clauses(utils.MaxExecutionTimeHint).Table(m.table).Unscoped().Where("status = ?", status).Limit(1).Find(&witness)
	if dbTx.Error != nil {
		return nil, utils.ConvertMysqlErrToDbErr(dbTx.Error)
	} else if dbTx.RowsAffected == 0 {
		return nil, utils.DbErrNotFound
	}
	return witness, nil
}

func (m *defaultWitnessModel) GetAndUpdateBatchesWitnessByStatus(beforeStatus, afterStatus int64, count int32) (witness [](*BatchWitness), err error) {
	
	err = m.DB.Table(m.table).Transaction(func(tx *gorm.DB) error {
		// dbTx := tx.Where("status = ?", beforeStatus).Limit(int(count)).Clauses(clause.Locking{Strength: "UPDATE",  Options: "SKIP LOCKED"}).Find(&witness)
		dbTx := tx.Clauses(utils.MaxExecutionTimeHint).Debug().Where("status = ?", beforeStatus).Order("height asc").Limit(int(count)).Clauses(clause.Locking{Strength: "UPDATE", }).Find(&witness)

		if dbTx.Error != nil {
			return utils.ConvertMysqlErrToDbErr(dbTx.Error)
		} else if dbTx.RowsAffected == 0 {
			return utils.DbErrNotFound
		}

		updateObject := make(map[string]interface{})
		for _, w := range witness {
			updateObject["Status"] = afterStatus
			dbTx := tx.Debug().Where("height = ?", w.Height).Updates(&updateObject)

			if dbTx.Error != nil {
				return dbTx.Error
			}
		}
		return nil
	})
	return witness, err
}

func (m *defaultWitnessModel) GetAndUpdateBatchesWitnessByHeight(height int, beforeStatus, afterStatus int64) (witness [](*BatchWitness), err error) {
	err = m.DB.Table(m.table).Transaction(func(tx *gorm.DB) error {
		// dbTx := tx.Where("status = ?", beforeStatus).Limit(int(count)).Clauses(clause.Locking{Strength: "UPDATE",  Options: "SKIP LOCKED"}).Find(&witness)
		dbTx := tx.Clauses(utils.MaxExecutionTimeHint).Where("height = ? and status = ?", height, beforeStatus).Order("height asc").Find(&witness)

		if dbTx.Error != nil {
			return utils.ConvertMysqlErrToDbErr(dbTx.Error)
		} else if dbTx.RowsAffected == 0 {
			return utils.DbErrNotFound
		}

		updateObject := make(map[string]interface{})
		for _, w := range witness {
			updateObject["Status"] = afterStatus
			dbTx := tx.Debug().Where("height = ?", w.Height).Updates(&updateObject)

			if dbTx.Error != nil {
				return dbTx.Error
			}
		}
		return nil
	})
	return witness, err
}

func (m *defaultWitnessModel) GetBatchWitnessByHeight(height int64) (witness *BatchWitness, err error) {
	dbTx := m.DB.Clauses(utils.MaxExecutionTimeHint).Table(m.table).Where("height = ?", height).Limit(1).Find(&witness)
	if dbTx.Error != nil {
		return nil, utils.ConvertMysqlErrToDbErr(dbTx.Error)
	} else if dbTx.RowsAffected == 0 {
		return nil, utils.DbErrNotFound
	}
	return witness, nil
}

func (m *defaultWitnessModel) CreateBatchWitness(witness []BatchWitness) error {
	//if witness.Height > 1 {
	//	_, err := m.GetBatchWitnessByHeight(witness.Height - 1)
	//	if err != nil {
	//		return fmt.Errorf("previous witness does not exist")
	//	}
	//}

	dbTx := m.DB.Table(m.table).Create(witness)
	if dbTx.Error != nil {
		return dbTx.Error
	}
	return nil
}

func (m *defaultWitnessModel) GetAllBatchHeightsByStatus(status int64, limit int, offset int) (witnessHeights []int64, err error) {
	dbTx := m.DB.Clauses(utils.MaxExecutionTimeHint).Table(m.table).Debug().Select("height").Where("status = ?", status).Offset(offset).Limit(limit).Find(&witnessHeights)
	if dbTx.Error != nil {
		return nil, utils.ConvertMysqlErrToDbErr(dbTx.Error)
	} else if dbTx.RowsAffected == 0 {
		return nil, utils.DbErrNotFound
	}
	return witnessHeights, nil
}

func (m *defaultWitnessModel) UpdateBatchWitnessStatus(witness *BatchWitness, status int64) error {
	dbTx := m.DB.Table(m.table).Where("height = ?", witness.Height).Updates(BatchWitness{
		Model: gorm.Model{
			UpdatedAt: time.Now(),
		},
		Status: status,
	})
	return dbTx.Error
}

func (m *defaultWitnessModel) GetRowCounts() (counts []int64, err error) {
	var count int64
	dbTx := m.DB.Clauses(utils.MaxExecutionTimeHint).Table(m.table).Count(&count)
	if dbTx.Error != nil {
		return nil, utils.ConvertMysqlErrToDbErr(dbTx.Error)
	}
	counts = append(counts, count)
	var publishedCount int64

	dbTx = m.DB.Clauses(utils.MaxExecutionTimeHint).Table(m.table).Debug().Where("status = ?", StatusPublished).Count(&publishedCount)
	if dbTx.Error != nil {
		return nil, utils.ConvertMysqlErrToDbErr(dbTx.Error)
	}
	counts = append(counts, publishedCount)

	var pendingCount int64
	dbTx = m.DB.Clauses(utils.MaxExecutionTimeHint).Table(m.table).Debug().Where("status = ?", StatusReceived).Count(&pendingCount)
	if dbTx.Error != nil {
		return nil, utils.ConvertMysqlErrToDbErr(dbTx.Error)
	}
	counts = append(counts, pendingCount)

	var finishedCount int64
	dbTx = m.DB.Clauses(utils.MaxExecutionTimeHint).Table(m.table).Debug().Where("status = ?", StatusFinished).Count(&finishedCount)
	if dbTx.Error != nil {
		return nil, utils.ConvertMysqlErrToDbErr(dbTx.Error)
	}
	counts = append(counts, finishedCount)
	return counts, nil
}
