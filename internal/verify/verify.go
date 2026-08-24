package verify

import (
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"auditlog/internal/chain"
	"auditlog/internal/model"
)

// BlockLister 抽象块列举与读取能力。
type BlockLister interface {
	ListBlockIDs() ([]uint64, error)
	ReadBlock(id uint64) (*model.Block, error)
}

// Verifier 沿哈希链逐块校验完整性。
type Verifier struct {
	chain  *chain.Chain
	reader BlockLister
	logger *log.Logger
}

// NewVerifier 创建校验器。
func NewVerifier(ch *chain.Chain, reader BlockLister, logger *log.Logger) (*Verifier, error) {
	if ch == nil {
		return nil, errors.New("verify chain is required")
	}
	if reader == nil {
		return nil, errors.New("verify reader is required")
	}
	if logger == nil {
		logger = log.New(os.Stderr, "[verify] ", log.LstdFlags)
	}
	return &Verifier{chain: ch, reader: reader, logger: logger}, nil
}

// VerifyAll 校验全部日志块。
func (v *Verifier) VerifyAll() (*Report, error) {
	ids, err := v.reader.ListBlockIDs()
	if err != nil {
		return nil, err
	}
	blocks := make([]*model.Block, 0, len(ids))
	for _, id := range ids {
		block, err := v.reader.ReadBlock(id)
		if err != nil {
			return nil, fmt.Errorf("load block %d for verification: %w", id, err)
		}
		blocks = append(blocks, block)
	}
	report := &Report{Checked: len(blocks), RanAt: time.Now().UTC(), Passed: true}
	if len(blocks) == 0 {
		return report, nil
	}
	_, broken, aggregateErr := v.chain.VerifyAll(blocks)
	for _, block := range blocks {
		if !v.chain.Knows(block.ID) {
			report.Broken = append(report.Broken, Break{BlockID: block.ID, Reason: "block is not registered in chain"})
		}
	}
	for _, err := range broken {
		var chainErr *chain.ChainError
		reason := err.Error()
		if errors.As(err, &chainErr) {
			reason = chainErr.Err.Error()
		}
		blockID := uint64(0)
		if chainErr != nil {
			blockID = chainErr.BlockID
		}
		report.Broken = append(report.Broken, Break{BlockID: blockID, Reason: reason})
	}
	if aggregateErr != nil || len(report.Broken) > 0 {
		report.Passed = false
	}
	var allRecords []model.Record
	for _, block := range blocks {
		allRecords = append(allRecords, block.Records...)
	}
	report.RecordsDigest = chain.RecordsHash(allRecords)
	return report, nil
}

// VerifyRange 校验指定区间的块。
func (v *Verifier) VerifyRange(from, to uint64) (*Report, error) {
	report := &Report{Checked: 0, RanAt: time.Now().UTC(), Passed: true}
	if from > to {
		return report, nil
	}
	var previous *model.Block
	for id := from; id <= to; id++ {
		block, err := v.reader.ReadBlock(id)
		if err != nil {
			return nil, err
		}
		report.Checked++
		if err := v.chain.VerifyLink(previous, block); err != nil {
			var chainErr *chain.ChainError
			reason := err.Error()
			if errors.As(err, &chainErr) {
				reason = chainErr.Err.Error()
			}
			report.Broken = append(report.Broken, Break{BlockID: block.ID, Reason: reason})
			report.Passed = false
			continue
		}
		previous = block
	}
	return report, nil
}
