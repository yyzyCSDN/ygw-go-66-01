package index

import (
	"reflect"
	"testing"
)

func TestBucketAddSortedUnique(t *testing.T) {
	bucket := &Bucket{Key: "k"}
	for _, seq := range []uint64{5, 1, 3, 3, 2} {
		bucket.Add(seq)
	}
	want := []uint64{1, 2, 3, 5}
	if !reflect.DeepEqual(bucket.Seqs, want) {
		t.Fatalf("bucket seqs = %v, want %v", bucket.Seqs, want)
	}
}

func TestIntersectAndUnion(t *testing.T) {
	first := []uint64{1, 3, 5, 7}
	second := []uint64{3, 4, 5}
	if got := Intersect(first, second); !reflect.DeepEqual(got, []uint64{3, 5}) {
		t.Fatalf("intersect = %v", got)
	}
	if got := Union(first, second); !reflect.DeepEqual(got, []uint64{1, 3, 4, 5, 7}) {
		t.Fatalf("union = %v", got)
	}
}

func TestBucketMerge(t *testing.T) {
	base := &Bucket{Key: "k", Seqs: []uint64{1, 4}}
	other := &Bucket{Key: "k", Seqs: []uint64{2, 4}}
	base.Merge(other)
	want := []uint64{1, 2, 4}
	if !reflect.DeepEqual(base.Seqs, want) {
		t.Fatalf("merged = %v, want %v", base.Seqs, want)
	}
}
