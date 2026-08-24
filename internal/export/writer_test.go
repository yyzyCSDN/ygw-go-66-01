package export

import (
	"strings"
	"testing"
	"time"

	"auditlog/internal/model"
)

func TestExportLineRoundTrip(t *testing.T) {
	rec := model.Record{
		Seq:       17,
		WrittenAt: time.Date(2026, 8, 24, 14, 5, 6, 7, time.UTC),
		Actor:     "svc",
		Action:    "config.change",
		Detail:    "listen addr 0.0.0.0:8090, note: keep",
	}
	line := FormatLine(rec)
	parsed, err := ParseExportLine(line)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if parsed != rec {
		t.Fatalf("round trip mismatch: %+v vs %+v", parsed, rec)
	}
}

func TestFormatExportTrailingNewline(t *testing.T) {
	records := []model.Record{
		{Seq: 1, WrittenAt: time.Now(), Actor: "a", Action: "x", Detail: "d"},
		{Seq: 2, WrittenAt: time.Now(), Actor: "b", Action: "y", Detail: "e"},
	}
	data := FormatExport(records)
	if !strings.HasSuffix(string(data), "\n") {
		t.Fatal("export output must end with newline")
	}
	if strings.Count(string(data), "\n") != 2 {
		t.Fatalf("line count = %d", strings.Count(string(data), "\n"))
	}
}
