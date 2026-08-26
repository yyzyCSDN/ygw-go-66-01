package reten

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"auditlog/internal/archive"
	"auditlog/internal/chain"
	"auditlog/internal/model"
	"auditlog/internal/store"
)

// Policy 描述日志保留策略：保留最近 KeepFor 时长的日志。
type Policy struct {
	KeepFor time.Duration
}

// ParsePolicy 解析形如 "30d"、"7d"、"24h"、"90m" 的保留时长。
func ParsePolicy(raw string) (Policy, error) {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return Policy{}, errors.New("retention policy is empty")
	}
	unit := raw[len(raw)-1]
	numberText := raw[:len(raw)-1]
	number, err := strconv.ParseInt(numberText, 10, 64)
	if err != nil || number <= 0 {
		return Policy{}, fmt.Errorf("invalid retention policy %q", raw)
	}
	var duration time.Duration
	switch unit {
	case 'd':
		duration = time.Duration(number) * 24 * time.Hour
	case 'h':
		duration = time.Duration(number) * time.Hour
	case 'm':
		duration = time.Duration(number) * time.Minute
	default:
		return Policy{}, fmt.Errorf("unsupported retention unit %q", unit)
	}
	return Policy{KeepFor: duration}, nil
}

// Cutoff 返回保留窗口的截止时间：早于该时间的日志视为过期。
func (p Policy) Cutoff(now time.Time) time.Time {
	return now.Add(-p.KeepFor)
}

// Report 汇总一次保留清理的执行结果。
type Report struct {
	Policy          Policy
	Cutoff          time.Time
	Scanned         int
	Kept            int
	Deleted         int
	SkippedWindow   int
	SkippedArchived int
}

// BlockDeleter 抽象块删除能力，由追加器实现，删除时同步清理头块状态。
type BlockDeleter interface {
	DeleteBlock(id uint64) (bool, error)
}

// Retention 负责按保留策略清理过期日志块。
type Retention struct {
	store    *store.FileStore
	archiver *archive.Archiver
	deleter  BlockDeleter
	logger   *log.Logger
	now      func() time.Time
}

// NewRetention 创建保留策略执行器。
func NewRetention(st *store.FileStore, ar *archive.Archiver, deleter BlockDeleter, logger *log.Logger) *Retention {
	if logger == nil {
		logger = log.New(os.Stderr, "[reten] ", log.LstdFlags)
	}
	return &Retention{
		store:    st,
		archiver: ar,
		deleter:  deleter,
		logger:   logger,
		now:      time.Now,
	}
}

// Apply 扫描全部块并删除过期块，返回执行报告。
func (r *Retention) Apply(policy Policy) (Report, error) {
	report := Report{Policy: policy, Cutoff: policy.Cutoff(r.now())}
	ids, err := r.listBlocks()
	if err != nil {
		return report, err
	}
	report.Scanned = len(ids)
	for _, id := range ids {
		block, err := r.loadBlock(id)
		if err != nil {
			return report, err
		}
		if block == nil {
			continue
		}
		if r.archiver.InActiveWindow(id) {
			report.SkippedWindow++
			continue
		}
		if !r.Expired(block, policy) {
			report.Kept++
			continue
		}
		if r.deleter == nil {
			if err := r.deleteBlock(id); err != nil {
				return report, fmt.Errorf("delete expired block %d: %w", id, err)
			}
		} else if _, err := r.deleter.DeleteBlock(id); err != nil {
			return report, fmt.Errorf("delete expired block %d: %w", id, err)
		}
		report.Deleted++
		r.logger.Printf("expired block %d (created %s, %d records, state %s)",
			id, block.CreatedAt.Format(time.RFC3339), block.RecordCount(), chain.StateLabel(block.State))
	}
	return report, nil
}

// Expired 判断块是否已超过保留窗口且当前状态允许清理。
func (r *Retention) Expired(block *model.Block, policy Policy) bool {
	if block == nil {
		return false
	}
	return block.CreatedAt.Before(r.now())
}

