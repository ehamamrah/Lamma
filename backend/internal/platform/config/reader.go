package config

import (
	"fmt"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"time"
)

// reader reads typed values from the environment and accumulates errors so
// all configuration problems are reported at once.
type reader struct {
	lookup func(string) (string, bool)
	errs   []error
}

func newReader(lookup func(string) (string, bool)) *reader {
	if lookup == nil {
		lookup = os.LookupEnv
	}
	return &reader{lookup: lookup}
}

func (r *reader) fail(key string, err error) {
	r.errs = append(r.errs, fmt.Errorf("%s: %w", key, err))
}

func (r *reader) str(key, def string) string {
	if v, ok := r.lookup(key); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return def
}

func (r *reader) required(key string) string {
	v := r.str(key, "")
	if v == "" {
		r.fail(key, fmt.Errorf("is required"))
	}
	return v
}

func (r *reader) integer(key string, def int) int {
	raw := r.str(key, "")
	if raw == "" {
		return def
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		r.fail(key, err)
		return def
	}
	return v
}

func (r *reader) boolean(key string, def bool) bool {
	raw := r.str(key, "")
	if raw == "" {
		return def
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		r.fail(key, err)
		return def
	}
	return v
}

func (r *reader) duration(key string, def time.Duration) time.Duration {
	raw := r.str(key, "")
	if raw == "" {
		return def
	}
	v, err := time.ParseDuration(raw)
	if err != nil || v <= 0 {
		r.fail(key, fmt.Errorf("invalid positive duration %q", raw))
		return def
	}
	return v
}

func (r *reader) list(key string) []string {
	var out []string
	for _, part := range strings.Split(r.str(key, ""), ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func (r *reader) prefixes(key, def string) []netip.Prefix {
	raw := r.list(key)
	if len(raw) == 0 && def != "" {
		raw = []string{def}
	}
	out := make([]netip.Prefix, 0, len(raw))
	for _, s := range raw {
		p, err := netip.ParsePrefix(s)
		if err != nil {
			r.fail(key, err)
			continue
		}
		out = append(out, p.Masked())
	}
	return out
}

func (r *reader) rate(key, def string) Rate {
	rate, err := ParseRate(r.str(key, def))
	if err != nil {
		r.fail(key, err)
		rate, _ = ParseRate(def)
	}
	return rate
}
