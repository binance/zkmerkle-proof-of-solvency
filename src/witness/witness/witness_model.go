package witness

import (
	"github.com/binance/zkmerkle-proof-of-solvency/src/utils"
	"time"

	"gorm.io/gorm"
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
		Status      int64
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
	var row *BatchWitness
	dbTx := m.DB.Table(m.table).Order("height desc").Limit(1).Find(&row)
	if dbTx.Error != nil {
		return 0, utils.DbErrSqlOperation
	} else if dbTx.RowsAffected == 0 {
		return 0, utils.DbErrNotFound
	}
	return row.Height, nil
}

func (m *defaultWitnessModel) GetLatestBatchWitness() (witness *BatchWitness, err error) {
	dbTx := m.DB.Table(m.table).Order("height desc").Limit(1).Find(&witness)
	if dbTx.Error != nil {
		return nil, dbTx.Error
	} else if dbTx.RowsAffected == 0 {
		return nil, utils.DbErrNotFound
	}
	return witness, nil
}

func (m *defaultWitnessModel) GetLatestBatchWitnessByStatus(status int64) (witness *BatchWitness, err error) {
	dbTx := m.DB.Table(m.table).Where("status = ?", status).Order("height asc").Limit(1).Find(&witness)
	if dbTx.Error != nil {
		return nil, utils.DbErrSqlOperation
	} else if dbTx.RowsAffected == 0 {
		return nil, utils.DbErrNotFound
	}
	return witness, nil
}

func (m *defaultWitnessModel) GetBatchWitnessByHeight(height int64) (witness *BatchWitness, err error) {
	dbTx := m.DB.Table(m.table).Where("height = ?", height).Limit(1).Find(&witness)
	if dbTx.Error != nil {
		return nil, utils.DbErrSqlOperation
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
		return utils.DbErrSqlOperation
	}
	return nil
}

func (m *defaultWitnessModel) UpdateBatchWitnessStatus(witness *BatchWitness, status int64) error {
	witness.Status = status
	witness.UpdatedAt = time.Now()
	dbTx := m.DB.Table(m.table).Save(witness)
	if dbTx.Error != nil {
		return utils.DbErrSqlOperation
	}
	return nil
}

func (m *defaultWitnessModel) GetRowCounts() (counts []int64, err error) {
	var count int64
	dbTx := m.DB.Table(m.table).Count(&count)
	if dbTx.Error != nil {
		return nil, dbTx.Error
	}
	counts = append(counts, count)
	var publishedCount int64
	dbTx = m.DB.Table(m.table).Where("status = ?", StatusPublished).Count(&publishedCount)
	if dbTx.Error != nil {
		return nil, dbTx.Error
	}
	counts = append(counts, publishedCount)

	var pendingCount int64
	dbTx = m.DB.Table(m.table).Where("status = ?", StatusReceived).Count(&pendingCount)
	if dbTx.Error != nil {
		return nil, dbTx.Error
	}
	counts = append(counts, pendingCount)

	var finishedCount int64
	dbTx = m.DB.Table(m.table).Where("status = ?", StatusFinished).Count(&finishedCount)
	if dbTx.Error != nil {
		return nil, dbTx.Error
	}
	counts = append(counts, finishedCount)
	return counts, nil
}
