package model

import (
	"encoding/json"
	"math"
	"testing"
)

func TestAPUplinkStatsPreservesCounterPresenceAndPrecision(t *testing.T) {
	var stats APUplinkStats
	if err := json.Unmarshal([]byte(`{"upPackets":18446744073709551615,"downPackets":9007199254740993,"upDropPackets":0,"downDropPackets":null,"upErrorsPackets":12}`), &stats); err != nil {
		t.Fatal(err)
	}
	if stats.UpPackets == nil || *stats.UpPackets != math.MaxUint64 {
		t.Fatalf("upPackets = %v, want exact MaxUint64", stats.UpPackets)
	}
	if stats.DownPackets == nil || *stats.DownPackets != 9007199254740993 {
		t.Fatalf("downPackets = %v, want exact value above 2^53", stats.DownPackets)
	}
	if stats.UpDropPackets == nil || *stats.UpDropPackets != 0 {
		t.Fatalf("explicit zero drop counter = %v, want non-nil zero", stats.UpDropPackets)
	}
	if stats.DownDropPackets != nil || stats.DownErrorPackets != nil {
		t.Fatal("missing/null counters must remain nil")
	}
	if stats.UpErrorPackets == nil || *stats.UpErrorPackets != 12 {
		t.Fatalf("upErrorsPackets = %v, want 12", stats.UpErrorPackets)
	}
	if !stats.HasCounters() {
		t.Fatal("populated counter data must be available")
	}
}

func TestAPUplinkStatsRejectsInvalidCounters(t *testing.T) {
	for _, field := range []string{"upPackets", "downPackets", "upDropPackets", "downDropPackets", "upErrorsPackets", "downErrorsPackets"} {
		for _, value := range []string{"-1", "1.5", "18446744073709551616", `"12"`} {
			t.Run(field+"/"+value, func(t *testing.T) {
				var stats APUplinkStats
				if err := json.Unmarshal([]byte(`{"`+field+`":`+value+`}`), &stats); err == nil {
					t.Fatalf("accepted invalid counter %s=%s", field, value)
				}
			})
		}
	}
}

func TestAPUplinkStatsHasCounters(t *testing.T) {
	var missing *APUplinkStats
	if missing.HasCounters() || (&APUplinkStats{}).HasCounters() {
		t.Fatal("nil/empty uplink must not report counters")
	}
	var zero uint64
	for _, stats := range []*APUplinkStats{
		{UpPackets: &zero}, {DownPackets: &zero},
		{UpDropPackets: &zero}, {DownDropPackets: &zero},
		{UpErrorPackets: &zero}, {DownErrorPackets: &zero},
	} {
		if !stats.HasCounters() {
			t.Fatal("an explicit zero counter must be available")
		}
	}
}
