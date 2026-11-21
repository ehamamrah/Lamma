package clientip

import (
	"net/http/httptest"
	"net/netip"
	"testing"
)

func TestResolve(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("172.30.0.10/32")}
	tests := []struct {
		name   string
		remote string
		xff    string
		want   string
	}{
		{"direct peer ignores spoofed header", "203.0.113.9:5000", "1.2.3.4", "203.0.113.9"},
		{"trusted proxy uses forwarded client", "172.30.0.10:4000", "198.51.100.7", "198.51.100.7"},
		{"rightmost untrusted hop wins over forged left", "172.30.0.10:4000", "6.6.6.6, 198.51.100.7", "198.51.100.7"},
		{"trusted proxy without header falls back to peer", "172.30.0.10:4000", "", "172.30.0.10"},
		{"garbage header falls back to peer", "172.30.0.10:4000", "not-an-ip", "172.30.0.10"},
		{"ipv4-mapped ipv6 is unmapped", "[::ffff:203.0.113.9]:1", "", "203.0.113.9"},
	}
	res := NewResolver(trusted)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			r.RemoteAddr = tt.remote
			if tt.xff != "" {
				r.Header.Set("X-Forwarded-For", tt.xff)
			}
			if got := res.Resolve(r).String(); got != tt.want {
				t.Fatalf("Resolve() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestKeyGroupsIPv6By64(t *testing.T) {
	a := Key(netip.MustParseAddr("2001:db8:1:2:aaaa::1"))
	b := Key(netip.MustParseAddr("2001:db8:1:2:bbbb::2"))
	c := Key(netip.MustParseAddr("2001:db8:1:3::1"))
	if a != b {
		t.Fatalf("same /64 must share a key: %s vs %s", a, b)
	}
	if a == c {
		t.Fatalf("different /64 must not share a key")
	}
	if got := Key(netip.MustParseAddr("192.0.2.1")); got != "192.0.2.1" {
		t.Fatalf("ipv4 key = %s", got)
	}
}

func TestHashIsKeyedAndStable(t *testing.T) {
	addr := netip.MustParseAddr("192.0.2.1")
	if string(Hash([]byte("k1"), addr)) != string(Hash([]byte("k1"), addr)) {
		t.Fatal("hash must be deterministic")
	}
	if string(Hash([]byte("k1"), addr)) == string(Hash([]byte("k2"), addr)) {
		t.Fatal("hash must depend on the secret")
	}
}
