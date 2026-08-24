package model

import (
	"testing"
	"time"
)

func TestBlockEncodeDecodeRoundTrip(t *testing.T) {
	block := &Block{
		ID:        7,
		PrevHash:  0x1234,
		Hash:      0xabcd,
		CreatedAt: time.Date(2026, 8, 24, 10, 0, 0, 0, time.UTC),
		Records: []Record{
			{Seq: 1, WrittenAt: time.Date(2026, 8, 24, 10, 0, 1, 0, time.UTC), Actor: "ops", Action: "start", Detail: "boot"},
			{Seq: 2, WrittenAt: time.Date(2026, 8, 24, 10, 0, 2, 0, time.UTC), Actor: "ops", Action: "stop", Detail: "halt"},
		},
		State: StateActive,
		File:  "blocks/00000000000000000007.blk",
	}
	decoded, err := DecodeBlock(block.Encode())
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.ID != block.ID || decoded.PrevHash != block.PrevHash || decoded.Hash != block.Hash {
		t.Fatalf("header mismatch: %+v", decoded)
	}
	if decoded.RecordCount() != 2 {
		t.Fatalf("record count = %d", decoded.RecordCount())
	}
	if decoded.Records[1].Detail != "halt" {
		t.Fatalf("record detail mismatch: %q", decoded.Records[1].Detail)
	}
	if err := decoded.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
}

func TestBlockValidateRejectsBadState(t *testing.T) {
	block := &Block{ID: 1, State: State("bogus")}
	if err := block.Validate(); err == nil {
		t.Fatal("expected validation error for bad state")
	}
}

func TestRecordEncodeDecodeRoundTrip(t *testing.T) {
	rec := Record{
		Seq:       42,
		WrittenAt: time.Date(2026, 8, 24, 11, 30, 0, 0, time.UTC),
		Actor:     "audit-bot",
		Action:    "config.change",
		Detail:    "rotation interval changed to 24h",
	}
	decoded, err := DecodeRecord(rec.Encode())
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded != rec {
		t.Fatalf("round trip mismatch: %+v vs %+v", decoded, rec)
	}
}

func TestRecordValidate(t *testing.T) {
	valid := Record{Seq: 1, Actor: "a", Action: "b", WrittenAt: time.Now()}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid record rejected: %v", err)
	}
	if err := (Record{Seq: 0, Actor: "a", Action: "b", WrittenAt: time.Now()}).Validate(); err == nil {
		t.Fatal("zero seq should be rejected")
	}
	if err := (Record{Seq: 1, Actor: "", Action: "b", WrittenAt: time.Now()}).Validate(); err == nil {
		t.Fatal("empty actor should be rejected")
	}
}

func TestBlockLastSeq(t *testing.T) {
	block := &Block{ID: 1, Records: []Record{{Seq: 3}, {Seq: 4}}}
	if block.LastSeq() != 4 {
		t.Fatalf("last seq = %d", block.LastSeq())
	}
	empty := &Block{ID: 2}
	if empty.LastSeq() != 0 {
		t.Fatalf("empty last seq = %d", empty.LastSeq())
	}
}
