package model

import (
	"testing"
	"time"
)

func TestExportCursorRoundTrip(t *testing.T) {
	cursor := ExportCursor{
		ExporterID: "exp-123",
		LastSeq:    99,
		BlockID:    5,
		IndexSeq:   120,
		UpdatedAt:  time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC),
		Complete:   true,
	}
	decoded, err := DecodeExportCursor(cursor.Encode())
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded != cursor {
		t.Fatalf("round trip mismatch: %+v", decoded)
	}
}

func TestJournalEntryRoundTrip(t *testing.T) {
	entry := JournalEntry{
		BlockID: 3,
		Action:  "archive",
		At:      time.Date(2026, 8, 24, 13, 0, 0, 0, time.UTC),
		Meta:    "window=2",
	}
	decoded, err := DecodeJournalEntry(entry.Encode())
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded != entry {
		t.Fatalf("round trip mismatch: %+v", decoded)
	}
}
