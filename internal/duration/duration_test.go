package duration

import (
	"testing"
	"time"
)

func TestParseDateDurations(t *testing.T) {
	tests := []struct {
		in   string
		want time.Duration
	}{
		{in: "P1Y", want: 365 * 24 * time.Hour},
		{in: "P1M", want: 30 * 24 * time.Hour},
		{in: "P2W", want: 14 * 24 * time.Hour},
		{in: "P3D", want: 3 * 24 * time.Hour},
		{in: "P1Y2M3W4D", want: (365 + 60 + 21 + 4) * 24 * time.Hour},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := Parse(tt.in)
			if err != nil {
				t.Fatalf("Parse(%q): %v", tt.in, err)
			}
			if got != tt.want {
				t.Fatalf("Parse(%q)=%s, want %s", tt.in, got, tt.want)
			}
		})
	}
}

func TestParseTimeDurations(t *testing.T) {
	tests := []struct {
		in   string
		want time.Duration
	}{
		{in: "PT1H", want: time.Hour},
		{in: "PT30M", want: 30 * time.Minute},
		{in: "PT45S", want: 45 * time.Second},
		{in: "P1DT2H3M4S", want: 24*time.Hour + 2*time.Hour + 3*time.Minute + 4*time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := Parse(tt.in)
			if err != nil {
				t.Fatalf("Parse(%q): %v", tt.in, err)
			}
			if got != tt.want {
				t.Fatalf("Parse(%q)=%s, want %s", tt.in, got, tt.want)
			}
		})
	}
}

func TestParseInvalidDurations(t *testing.T) {
	tests := []string{
		"",
		"30D",
		"P",
		"PT",
		"PX",
		"P1",
		"P1H",
		"PT1D",
	}
	for _, in := range tests {
		t.Run(in, func(t *testing.T) {
			if _, err := Parse(in); err == nil {
				t.Fatalf("Parse(%q): expected error", in)
			}
		})
	}
}
