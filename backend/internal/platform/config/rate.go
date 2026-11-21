package config

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Rate is "Count requests per Period", e.g. "120/m" or "5/15m".
type Rate struct {
	Count  int
	Period time.Duration
}

var unitPeriods = map[string]time.Duration{
	"s": time.Second,
	"m": time.Minute,
	"h": time.Hour,
}

// ParseRate parses "N/unit" or "N/duration" (units: s, m, h).
func ParseRate(s string) (Rate, error) {
	countRaw, periodRaw, ok := strings.Cut(strings.TrimSpace(s), "/")
	if !ok {
		return Rate{}, fmt.Errorf("rate %q: want N/period", s)
	}
	count, err := strconv.Atoi(countRaw)
	if err != nil || count <= 0 {
		return Rate{}, fmt.Errorf("rate %q: invalid count", s)
	}
	period, err := parsePeriod(periodRaw)
	if err != nil {
		return Rate{}, fmt.Errorf("rate %q: %w", s, err)
	}
	return Rate{Count: count, Period: period}, nil
}

func parsePeriod(s string) (time.Duration, error) {
	if d, ok := unitPeriods[s]; ok {
		return d, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("invalid period %q", s)
	}
	return d, nil
}
