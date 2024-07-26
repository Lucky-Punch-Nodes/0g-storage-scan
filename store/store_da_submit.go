package store

import (
	"time"

	"github.com/0glabs/0g-storage-scan/contract"

	"github.com/Conflux-Chain/go-conflux-util/store/mysql"
	"github.com/openweb3/web3go/types"
	"gorm.io/gorm"
)

type DASubmit struct {
	Epoch    uint64 `gorm:"primaryKey;autoIncrement:false"`
	QuorumID uint64 `gorm:"primaryKey;autoIncrement:false"`
	RootHash string `gorm:"primaryKey;autoIncrement:false;size:66;index:idx_root"`
	Verified bool   `gorm:"not null;default:false"`

	BlockNumber         uint64    `gorm:"not null;index:idx_bn"`
	BlockTime           time.Time `gorm:"not null;index:idx_bt"`
	TxHash              string    `gorm:"size:66;not null;index:idx_txHash,length:10"`
	BlockNumberVerified uint64    `gorm:"not null;index:idx_bn_verified"`
	BlockTimeVerified   time.Time `gorm:"not null;index:idx_bt_verified"`
	TxHashVerified      string    `gorm:"size:66;not null;index:idx_txHash_verified,length:10"`
}

func NewDASubmit(blockTime time.Time, log types.Log, filter *contract.DAEntranceFilterer) (*DASubmit, error) {
	dataUpload, err := filter.ParseDataUpload(*log.ToEthLog())
	if err != nil {
		return nil, err
	}

	submit := &DASubmit{
		Epoch:    dataUpload.Epoch.Uint64(),
		QuorumID: dataUpload.QuorumId.Uint64(),
		RootHash: string(dataUpload.DataRoot[:]),

		BlockNumber: log.BlockNumber,
		BlockTime:   blockTime,
		TxHash:      log.TxHash.String(),
	}

	return submit, nil
}

func NewDASubmitVerified(blockTime time.Time, log types.Log, filter *contract.DAEntranceFilterer) (*DASubmit, error) {
	commitVerified, err := filter.ParseErasureCommitmentVerified(*log.ToEthLog())
	if err != nil {
		return nil, err
	}

	submit := &DASubmit{
		Epoch:    commitVerified.Epoch.Uint64(),
		QuorumID: commitVerified.QuorumId.Uint64(),
		RootHash: string(commitVerified.DataRoot[:]),

		Verified:            true,
		BlockNumberVerified: log.BlockNumber,
		BlockTimeVerified:   blockTime,
		TxHashVerified:      log.TxHash.String(),
	}

	return submit, nil
}

func (DASubmit) TableName() string {
	return "da_submits"
}

type DASubmitStore struct {
	*mysql.Store
}

func newDASubmitStore(db *gorm.DB) *DASubmitStore {
	return &DASubmitStore{
		Store: mysql.NewStore(db),
	}
}

func (ss *DASubmitStore) Add(dbTx *gorm.DB, submits []DASubmit) error {
	return dbTx.CreateInBatches(submits, batchSizeInsert).Error
}

func (ss *DASubmitStore) Pop(dbTx *gorm.DB, block uint64) error {
	return dbTx.Where("block_number >= ?", block).Delete(&DASubmit{}).Error
}

func (ss *DASubmitStore) UpdateByPrimaryKey(dbTx *gorm.DB, s DASubmit) error {
	db := ss.DB
	if dbTx != nil {
		db = dbTx
	}

	if err := db.Model(&s).Where("epoch=? and quorum_id=? and root_hash=?", s.Epoch, s.QuorumID, s.RootHash).
		Updates(s).Error; err != nil {
		return err
	}

	return nil
}
