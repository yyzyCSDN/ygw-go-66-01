package search

import (
	"time"

	"auditlog/internal/model"
)

// ApplyFilters 在内存中对记录列表应用条件过滤，保持原顺序。
func ApplyFilters(records []model.Record, actor, action, keyword string, from, to time.Time) []model.Record {
	output := make([]model.Record, 0, len(records))
	for _, rec := range records {
		if Matches(rec, actor, action, keyword, from, to) {
			output = append(output, rec)
		}
	}
	return output
}
