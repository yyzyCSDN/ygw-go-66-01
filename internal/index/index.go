package index

import (
	"errors"
	"sort"
	"sync"
	"time"

	"auditlog/internal/model"
)

// Query 描述一次检索的条件。空字段表示不限制该维度。
type Query struct {
	Actor   string
	Action  string
	Keyword string
	From    time.Time
	To      time.Time
}

// Index 维护审计记录的倒排索引与时间线。
type Index struct {
	mu         sync.RWMutex
	actors     map[string]*Bucket
	actions    map[string]*Bucket
	keywords   map[string]*Bucket
	timeline   []TimedSeq
	total      int
	generation uint64
}

// NewIndex 创建空索引。
func NewIndex() *Index {
	return &Index{
		actors:   make(map[string]*Bucket),
		actions:  make(map[string]*Bucket),
		keywords: make(map[string]*Bucket),
	}
}

// Add 将一条记录写入倒排索引并推进时间线。
func (x *Index) Add(rec model.Record) error {
	if rec.Seq == 0 {
		return errors.New("cannot index a record without seq")
	}
	x.mu.Lock()
	defer x.mu.Unlock()
	if x.actors[rec.Actor] == nil {
		x.actors[rec.Actor] = &Bucket{Key: rec.Actor}
	}
	x.actors[rec.Actor].Add(rec.Seq)
	if x.actions[rec.Action] == nil {
		x.actions[rec.Action] = &Bucket{Key: rec.Action}
	}
	x.actions[rec.Action].Add(rec.Seq)
	for _, word := range rec.Keywords() {
		if x.keywords[word] == nil {
			x.keywords[word] = &Bucket{Key: word}
		}
		x.keywords[word].Add(rec.Seq)
	}
	x.timeline = append(x.timeline, TimedSeq{At: rec.WrittenAt, Seq: rec.Seq})
	x.total++
	x.generation++
	return nil
}

// Query 返回满足条件的记录序号（升序）与命中总数。
func (x *Index) Query(q Query) ([]uint64, int, error) {
	hits, err := x.resolve(q)
	if err != nil {
		return nil, 0, err
	}
	return hits, len(hits), nil
}

// QueryCount 返回满足条件的命中总数，检索组件用它对齐分页边界。
func (x *Index) QueryCount(q Query) (int, error) {
	hits, err := x.resolve(q)
	if err != nil {
		return 0, err
	}
	return len(hits), nil
}

// resolve 计算查询命中的记录序号集合。
func (x *Index) resolve(q Query) ([]uint64, error) {
	x.mu.RLock()
	defer x.mu.RUnlock()
	var result []uint64
	first := true
	combine := func(bucket *Bucket) {
		if bucket == nil {
			return
		}
		if first {
			result = append(result, bucket.Seqs...)
			first = false
			return
		}
		result = Intersect(result, bucket.Seqs)
	}
	if q.Actor != "" {
		combine(x.actors[q.Actor])
	}
	if q.Action != "" {
		combine(x.actions[q.Action])
	}
	if q.Keyword != "" {
		combine(x.keywords[q.Keyword])
	}
	if first {
		result = make([]uint64, 0, x.total)
		for _, item := range x.timeline {
			result = append(result, item.Seq)
		}
	}
	if !q.From.IsZero() || !q.To.IsZero() {
		result = filterByTime(result, x.timeline, q.From, q.To)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result, nil
}

// filterByTime 按时间线过滤命中序号。
func filterByTime(seqs []uint64, timeline []TimedSeq, from, to time.Time) []uint64 {
	bySeq := make(map[uint64]time.Time, len(timeline))
	for _, item := range timeline {
		bySeq[item.Seq] = item.At
	}
	output := seqs[:0]
	for _, seq := range seqs {
		at, ok := bySeq[seq]
		if !ok {
			continue
		}
		if !from.IsZero() && at.Before(from) {
			continue
		}
		if !to.IsZero() && !at.Before(to) {
			continue
		}
		output = append(output, seq)
	}
	return output
}

// Generation 返回索引当前代次，代次随每次写入递增。
func (x *Index) Generation() uint64 {
	x.mu.RLock()
	defer x.mu.RUnlock()
	return x.generation
}

// Total 返回索引中的记录总数。
func (x *Index) Total() int {
	x.mu.RLock()
	defer x.mu.RUnlock()
	return x.total
}

