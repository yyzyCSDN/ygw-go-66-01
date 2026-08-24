package store

import (
	"path/filepath"
	"testing"
)

func TestPathsLayout(t *testing.T) {
	paths := Paths{Root: filepath.Join("tmp", "audit")}
	if got := paths.BlockFile(3); got != filepath.Join("tmp", "audit", "blocks", "00000000000000000003.blk") {
		t.Fatalf("unexpected block path: %s", got)
	}
	if got := paths.ArchiveBlockFile(4); got != filepath.Join("tmp", "audit", "archive", "00000000000000000004.blk") {
		t.Fatalf("unexpected archive path: %s", got)
	}
	if got := paths.CursorFile("exp-1"); got != filepath.Join("tmp", "audit", "cursor", "exp-1.cur") {
		t.Fatalf("unexpected cursor path: %s", got)
	}
}

func TestParseBlockFileName(t *testing.T) {
	id, err := ParseBlockFileName("00000000000000000012.blk")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if id != 12 {
		t.Fatalf("id = %d", id)
	}
	if _, err := ParseBlockFileName("notes.txt"); err == nil {
		t.Fatal("expected parse error")
	}
}
