package model

import (
	"encoding/json"
	"testing"
)

func TestWANLatencySupportsHighAndFractionalMilliseconds(t *testing.T) {
	for _, tt := range []struct {
		payload string
		want    float64
	}{
		{`{"latency":250}`, 250},
		{`{"latency":1250.5}`, 1250.5},
		{`{"latency":0}`, 0},
		{`{"latency":-1}`, -1},
	} {
		t.Run(tt.payload, func(t *testing.T) {
			var wan Wan
			if err := json.Unmarshal([]byte(tt.payload), &wan); err != nil {
				t.Fatalf("decoding WAN status: %v", err)
			}
			if wan.Latency != tt.want {
				t.Fatalf("latency = %v, want %v", wan.Latency, tt.want)
			}
		})
	}
}
