package reten

import (
	"testing"
	"time"
)

func TestParsePolicy(t *testing.T) {
	cases := []struct {
		raw  string
		want time.Duration
	}{
		{"30d", 30 * 24 * time.Hour},
		{"7d", 7 * 24 * time.Hour},
		{"24h", 24 * time.Hour},
		{"90m", 90 * time.Minute},
	}
	for _, item := range cases {
		policy, err := ParsePolicy(item.raw)
		if err != nil {
			t.Fatalf("parse %q: %v", item.raw, err)
		}
		if policy.KeepFor != item.want {
			t.Fatalf("parse %q = %v", item.raw, policy.KeepFor)
		}
	}
}

func TestParsePolicyRejectsInvalid(t *testing.T) {
	for _, raw := range []string{"", "abc", "10x", "-3d", "0d"} {
		if _, err := ParsePolicy(raw); err == nil {
			t.Fatalf("expected error for %q", raw)
		}
	}
}

func TestPolicyCutoff(t *testing.T) {
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	policy, _ := ParsePolicy("7d")
	want := now.Add(-7 * 24 * time.Hour)
	if got := policy.Cutoff(now); !got.Equal(want) {
		t.Fatalf("cutoff = %v, want %v", got, want)
	}
}
