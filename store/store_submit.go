package store

import (
	"database/sql"
	"encoding/json"
	"math/big"
	"time"

	"github.com/0glabs/0g-storage-client/contract"
	"github.com/0glabs/0g-storage-client/core"
	"github.com/Conflux-Chain/go-conflux-util/store/mysql"
	"github.com/ethereum/go-ethereum/common"
	"github.com/openweb3/web3go/types"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type Status uint8

const (
	NotUploaded Status = iota
	Uploading
	Uploaded
)

type Submit struct {
	SubmissionIndex uint64 `gorm:"primaryKey;autoIncrement:false"`
	RootHash        string `gorm:"size:66;index:idx_root"`
	Sender          string `gorm:"-"`
	SenderID        uint64 `gorm:"not null"`
	Length          uint64 `gorm:"not null"`

	BlockNumber uint64    `gorm:"not null;index:idx_bn"`
	BlockTime   time.Time `gorm:"not null;index:idx_bt"`
	TxHash      string    `gorm:"size:66;not null;index:idx_txHash,length:10"`

	TotalSegNum    uint64          `gorm:"not null;default:0"`
	UploadedSegNum uint64          `gorm:"not null;default:0"`
	Status         uint8           `gorm:"not null;default:0"`
	Fee            decimal.Decimal `gorm:"type:decimal(65);not null"`

	Extra []byte `gorm:"type:mediumText"` // json field
}

type SubmitExtra struct {
	Identity   common.Hash         `json:"identity"`
	StartPos   *big.Int            `json:"startPos"`
	Submission contract.Submission `json:"submission"`
}

func NewSubmit(blockTime time.Time, log types.Log, filter *contract.FlowFilterer) (*Submit, error) {
	flowSubmit, err := filter.ParseSubmit(*log.ToEthLog())
	if err != nil {
		return nil, err
	}

	extra, err := json.Marshal(SubmitExtra{
		Identity:   flowSubmit.Identity,
		StartPos:   flowSubmit.StartPos,
		Submission: flowSubmit.Submission,
	})
	if err != nil {
		return nil, err
	}

	length := flowSubmit.Submission.Length.Uint64()
	submit := &Submit{
		SubmissionIndex: flowSubmit.SubmissionIndex.Uint64(),
		RootHash:        flowSubmit.Submission.Root().String(),
		Sender:          flowSubmit.Sender.String(),
		Length:          length,
		BlockNumber:     log.BlockNumber,
		BlockTime:       blockTime,
		TxHash:          log.TxHash.String(),
		Fee:             decimal.NewFromBigInt(flowSubmit.Submission.Fee(), 0),
		TotalSegNum:     (length-1)/core.DefaultSegmentSize + 1,
		Extra:           extra,
	}

	return submit, nil
}

func (Submit) TableName() string {
	return "submits"
}

type SubmitStore struct {
	*mysql.Store
}

func newSubmitStore(db *gorm.DB) *SubmitStore {
	return &SubmitStore{
		Store: mysql.NewStore(db),
	}
}

func (ss *SubmitStore) Add(dbTx *gorm.DB, submits []Submit) error {
	return dbTx.CreateInBatches(submits, batchSizeInsert).Error
}

func (ss *SubmitStore) Pop(dbTx *gorm.DB, block uint64) error {
	return dbTx.Where("block_number >= ?", block).Delete(&Submit{}).Error
}

func (ss *SubmitStore) Count(startTime, endTime time.Time) (*SubmitStatResult, error) {
	var result SubmitStatResult
	err := ss.DB.Model(&Submit{}).Select(`count(*) as file_count, 
		IFNULL(sum(length), 0) as data_size, IFNULL(sum(fee), 0) as base_fee, count(distinct tx_hash) as tx_count,
		count(distinct sender_id) as sender_count`).
		Where("block_time >= ? and block_time < ?", startTime, endTime).Find(&result).Error
	if err != nil {
		return nil, err
	}

	return &result, nil
}

func (ss *SubmitStore) UpdateByPrimaryKey(dbTx *gorm.DB, s *Submit) error {
	db := ss.DB
	if dbTx != nil {
		db = dbTx
	}

	if err := db.Model(&s).Where("submission_index=?", s.SubmissionIndex).
		Updates(s).Error; err != nil {
		return err
	}

	return nil
}

func (ss *SubmitStore) List(rootHash *string, txHash *string, idDesc bool, skip, limit int) (int64, []Submit, error) {
	dbRaw := ss.DB.Model(&Submit{})
	var conds []func(db *gorm.DB) *gorm.DB
	if rootHash != nil {
		conds = append(conds, RootHash(*rootHash))
	}
	if txHash != nil {
		conds = append(conds, TxHash(*txHash))
	}
	dbRaw.Scopes(conds...)

	var orderBy string
	if idDesc {
		orderBy = "submission_index DESC"
	} else {
		orderBy = "submission_index ASC"
	}

	list := new([]Submit)

	if len(conds) == 0 {
		var maxId sql.NullInt64
		if err := ss.DB.Model(&Submit{}).Select("MAX(submission_index)").Find(&maxId).Error; err != nil {
			return 0, nil, err
		}
		if !maxId.Valid {
			return 0, nil, nil
		}

		if skip > 0 {
			if idDesc {
				dbRaw.Where("submission_index <= ?", maxId.Int64-int64(skip))
			} else {
				dbRaw.Where("submission_index > ?", skip)
			}
		}
		if err := dbRaw.Order(orderBy).Limit(limit).Find(list).Error; err != nil {
			return 0, nil, err
		}

		return maxId.Int64, *list, nil
	}

	total, err := ss.Store.ListByOrder(dbRaw, orderBy, skip, limit, list)
	if err != nil {
		return 0, nil, err
	}

	return total, *list, nil
}

func (ss *SubmitStore) BatchGetNotFinalized(submissionIndex uint64, batch int) ([]Submit, error) {
	submits := new([]Submit)
	if err := ss.DB.Raw("select submission_index, sender_id, total_seg_num from submits where submission_index >= ? and status < ? order by submission_index asc limit ?",
		submissionIndex, Uploaded, batch).Scan(submits).Error; err != nil {
		return nil, err
	}

	return *submits, nil
}
