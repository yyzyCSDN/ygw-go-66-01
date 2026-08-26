package index

import "sort"

// Bucket 是一个倒排桶：键为检索维度值，Seqs 为命中的记录序号（升序）。
type Bucket struct {
	Key  string
	Seqs []uint64
}

// Add 将序号加入桶，保持升序并去重。
func (b *Bucket) Add(seq uint64) {
	position := sort.Search(len(b.Seqs), func(i int) bool { return b.Seqs[i] >= seq })
	if position < len(b.Seqs) && b.Seqs[position] == seq {
		return
	}
	b.Seqs = append(b.Seqs, 0)
	copy(b.Seqs[position+1:], b.Seqs[position:])
	b.Seqs[position] = seq
}

// Merge 合并另一个桶的序号，结果保持升序去重。
func (b *Bucket) Merge(other *Bucket) {
	if other == nil {
		return
	}
	b.Seqs = Union(b.Seqs, other.Seqs)
}

// Intersect 返回两个升序序号列表的交集。
func Intersect(first, second []uint64) []uint64 {
	output := make([]uint64, 0)
	i, j := 0, 0
	for i < len(first) && j < len(second) {
		switch {
		case first[i] == second[j]:
			output = append(output, first[i])
			i++
			j++
		case first[i] < second[j]:
			i++
		default:
			j++
		}
	}
	return output
}

// Union 返回两个升序序号列表的并集。
func Union(first, second []uint64) []uint64 {
	output := make([]uint64, 0, len(first)+len(second))
	i, j := 0, 0
	for i < len(first) && j < len(second) {
		switch {
		case first[i] == second[j]:
			output = append(output, first[i])
			i++
			j++
		case first[i] < second[j]:
			output = append(output, first[i])
			i++
		default:
			output = append(output, second[j])
			j++
		}
	}
	output = append(output, first[i:]...)
	output = append(output, second[j:]...)
	return output
}
