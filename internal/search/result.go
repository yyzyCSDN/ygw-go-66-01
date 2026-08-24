package search

import (
	"time"

	"auditlog/internal/model"
)

// PageResult 是一次分页检索的返回结果。
type PageResult struct {
	Records []model.Record
	Total   int
	Page    int
	Size    int
	HasMore bool
}

// Matches 判断记录是否满足过滤条件；空条件视为不限制。
func Matches(rec model.Record, actor, action, keyword string, from, to time.Time) bool {
	if actor != "" && rec.Actor != actor {
		return false
	}
	if action != "" && rec.Action != action {
		return false
	}
	if keyword != "" && !containsKeyword(rec, keyword) {
		return false
	}
	if !from.IsZero() && rec.WrittenAt.Before(from) {
		return false
	}
	if !to.IsZero() && !rec.WrittenAt.Before(to) {
		return false
	}
	return true
}

func containsKeyword(rec model.Record, keyword string) bool {
	for _, word := range rec.Keywords() {
		if word == keyword {
			return true
		}
	}
	return false
}
