package verify

import (
	"fmt"
	"time"
)

// Break 描述链上一个断链位置。
type Break struct {
	BlockID uint64
	Reason  string
}

// Report 汇总一次完整性校验的结果。
type Report struct {
	Checked       int
	Broken        []Break
	Passed        bool
	RanAt         time.Time
	RecordsDigest uint64
}

// Merge 合并另一份校验报告。
func (r *Report) Merge(other *Report) *Report {
	if other == nil {
		return r
	}
	r.Checked += other.Checked
	r.Broken = append(r.Broken, other.Broken...)
	r.Passed = r.Passed && other.Passed
	return r
}

// Summary 返回报告的一句话摘要。
func (r *Report) Summary() string {
	if r == nil {
		return "no verification ran"
	}
	if r.Passed {
		return fmt.Sprintf("verified %d blocks, chain intact", r.Checked)
	}
	return fmt.Sprintf("verified %d blocks, %d broken link(s) found", r.Checked, len(r.Broken))
}
