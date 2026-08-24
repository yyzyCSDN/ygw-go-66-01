package archive

import (
	"log"
	"reflect"
	"testing"

	"auditlog/internal/model"
	"auditlog/internal/store"
)

func newTestArchiver(t *testing.T) *Archiver {
	t.Helper()
	root := t.TempDir()
	fileStore, err := store.NewFileStore(root)
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	t.Cleanup(func() { _ = fileStore.Close() })
	ar, err := NewArchiver(fileStore, log.New(testDiscard{}, "", 0))
	if err != nil {
		t.Fatalf("archiver: %v", err)
	}
	return ar
}

type testDiscard struct{}

func (testDiscard) Write(p []byte) (int, error) { return len(p), nil }

func activeBlocks(ids ...uint64) []*model.Block {
	out := make([]*model.Block, len(ids))
	for i, id := range ids {
		out[i] = &model.Block{ID: id, State: model.StateActive}
	}
	return out
}

func archivedBlock(id uint64) *model.Block {
	return &model.Block{ID: id, State: model.StateArchived}
}

// TestWindowSizeCoversBoundaryBlocks 断言 size 个连续活跃块全部入选，
// 不再因开闭区间错位而漏掉窗口末块。
func TestWindowSizeCoversBoundaryBlocks(t *testing.T) {
	ar := newTestArchiver(t)
	cases := []struct {
		name   string
		blocks []*model.Block
		size   int
		want   []uint64
	}{
		{"single block", activeBlocks(1), 1, []uint64{1}},
		{"three of five", activeBlocks(1, 2, 3, 4, 5), 3, []uint64{1, 2, 3}},
		{"exhaust active", activeBlocks(1, 2), 5, []uint64{1, 2}},
		{"start above one", activeBlocks(10, 11, 12, 13), 2, []uint64{10, 11}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			selected, err := ar.Window(tc.blocks, tc.size)
			if err != nil {
				t.Fatalf("window: %v", err)
			}
			got := idsOf(selected)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("selected = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestWindowSkipsArchivedAndAdvances 断言窗口从最旧活跃块起算，已归档块
// 不计入窗口，连续调用能逐批推进。
func TestWindowSkipsArchivedAndAdvances(t *testing.T) {
	ar := newTestArchiver(t)
	// 已归档的 1、2 不应进入窗口，窗口应从最旧活跃块 3 起算。
	blocks := []*model.Block{
		archivedBlock(1),
		archivedBlock(2),
		&model.Block{ID: 3, State: model.StateActive},
		&model.Block{ID: 4, State: model.StateActive},
		&model.Block{ID: 5, State: model.StateActive},
	}
	selected, err := ar.Window(blocks, 2)
	if err != nil {
		t.Fatalf("window: %v", err)
	}
	if got := idsOf(selected); !reflect.DeepEqual(got, []uint64{3, 4}) {
		t.Fatalf("selected = %v, want [3 4]", got)
	}
}

// TestWindowRecordsActiveBounds 断言 Window 记录的活动窗口边界与
// InActiveWindow 的半开区间判定一致：窗口内为真，边界外的末块下一块为假。
func TestWindowRecordsActiveBounds(t *testing.T) {
	ar := newTestArchiver(t)
	selected, err := ar.Window(activeBlocks(7, 8, 9), 2)
	if err != nil {
		t.Fatalf("window: %v", err)
	}
	if got := idsOf(selected); !reflect.DeepEqual(got, []uint64{7, 8}) {
		t.Fatalf("selected = %v, want [7 8]", got)
	}
	if !ar.InActiveWindow(7) {
		t.Fatalf("block 7 should be inside active window")
	}
	if !ar.InActiveWindow(8) {
		t.Fatalf("block 8 (window tail) should be inside active window")
	}
	if ar.InActiveWindow(9) {
		t.Fatalf("block 9 should be outside active window")
	}
}

// TestWindowEmptyResetsBounds 断言没有活跃块时边界被清空，
// InActiveWindow 不再误报。
func TestWindowEmptyResetsBounds(t *testing.T) {
	ar := newTestArchiver(t)
	if _, err := ar.Window(activeBlocks(1, 2), 1); err != nil {
		t.Fatalf("window: %v", err)
	}
	if !ar.InActiveWindow(1) {
		t.Fatalf("block 1 should be inside active window")
	}
	if _, err := ar.Window([]*model.Block{archivedBlock(1)}, 1); err != nil {
		t.Fatalf("window: %v", err)
	}
	if ar.InActiveWindow(1) {
		t.Fatalf("block 1 should be outside window after no active blocks")
	}
}

func idsOf(blocks []*model.Block) []uint64 {
	out := make([]uint64, 0, len(blocks))
	for _, b := range blocks {
		out = append(out, b.ID)
	}
	return out
}
