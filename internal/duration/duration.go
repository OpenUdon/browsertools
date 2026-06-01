// Package duration parses a subset of ISO-8601 duration strings into
// time.Duration values for use by the review and revalidate packages.
package duration

import (
	"fmt"
	"time"
)

// Parse parses a subset of ISO-8601 durations into a time.Duration.
// Supports P(n)Y, P(n)M, P(n)W, P(n)D and PT(n)H, PT(n)M, PT(n)S forms.
// Uses approximate values: 1Y=365d, 1M=30d.
// Returns an error for "P", "PT", or any string with no digit-unit pairs.
func Parse(s string) (time.Duration, error) {
	if len(s) == 0 || s[0] != 'P' {
		return 0, fmt.Errorf("invalid ISO-8601 duration %q", s)
	}
	var total time.Duration
	parsed := 0 // number of digit-unit pairs successfully parsed
	rest := s[1:]
	inTime := false
	if len(rest) > 0 && rest[0] == 'T' {
		inTime = true
		rest = rest[1:]
	}
	for len(rest) > 0 {
		if rest[0] == 'T' {
			inTime = true
			rest = rest[1:]
			continue
		}
		n := 0
		i := 0
		for i < len(rest) && rest[i] >= '0' && rest[i] <= '9' {
			n = n*10 + int(rest[i]-'0')
			i++
		}
		if i == 0 {
			return 0, fmt.Errorf("invalid ISO-8601 duration: expected digit before unit in %q", s)
		}
		if i >= len(rest) {
			return 0, fmt.Errorf("invalid ISO-8601 duration: digit(s) with no unit in %q", s)
		}
		unit := rest[i]
		rest = rest[i+1:]
		switch {
		case !inTime && unit == 'Y':
			total += time.Duration(n) * 365 * 24 * time.Hour
		case !inTime && unit == 'M':
			total += time.Duration(n) * 30 * 24 * time.Hour
		case !inTime && unit == 'W':
			total += time.Duration(n) * 7 * 24 * time.Hour
		case !inTime && unit == 'D':
			total += time.Duration(n) * 24 * time.Hour
		case inTime && unit == 'H':
			total += time.Duration(n) * time.Hour
		case inTime && unit == 'M':
			total += time.Duration(n) * time.Minute
		case inTime && unit == 'S':
			total += time.Duration(n) * time.Second
		default:
			return 0, fmt.Errorf("unknown ISO-8601 duration unit %q in %q", unit, s)
		}
		parsed++
	}
	if parsed == 0 {
		return 0, fmt.Errorf("invalid ISO-8601 duration %q: no designators found", s)
	}
	return total, nil
}
