// Package clientip resolves the real client address behind trusted proxies
// and derives rate-limit keys that resist address rotation.
package clientip

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// Resolver extracts client IPs, honouring X-Forwarded-For only from trusted peers.
type Resolver struct {
	trusted []netip.Prefix
}

// NewResolver creates a Resolver trusting the given proxy prefixes.
func NewResolver(trusted []netip.Prefix) *Resolver {
	return &Resolver{trusted: trusted}
}

// Resolve returns the client address for r.
func (res *Resolver) Resolve(r *http.Request) netip.Addr {
	peer := parseHostPort(r.RemoteAddr)
	if !res.isTrusted(peer) {
		return peer
	}
	return res.fromForwarded(r.Header.Values("X-Forwarded-For"), peer)
}

// fromForwarded walks X-Forwarded-For right-to-left and returns the first
// untrusted hop: entries left of it may be forged by the client.
func (res *Resolver) fromForwarded(headers []string, peer netip.Addr) netip.Addr {
	hops := strings.Split(strings.Join(headers, ","), ",")
	for i := len(hops) - 1; i >= 0; i-- {
		addr, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
		if err != nil {
			return peer
		}
		if !res.isTrusted(addr.Unmap()) {
			return addr.Unmap()
		}
	}
	return peer
}

func (res *Resolver) isTrusted(addr netip.Addr) bool {
	for _, p := range res.trusted {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

func parseHostPort(s string) netip.Addr {
	host, _, err := net.SplitHostPort(s)
	if err != nil {
		host = s
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return netip.IPv4Unspecified()
	}
	return addr.Unmap()
}

// Key returns the rate-limit identity: the full IPv4 address, or the /64
// prefix for IPv6 (one subscriber usually controls a whole /64).
func Key(addr netip.Addr) string {
	if addr.Is6() {
		p, _ := addr.Prefix(64)
		return p.String()
	}
	return addr.String()
}

// Hash returns an HMAC of the client key so no raw IPs are persisted.
func Hash(secret []byte, addr netip.Addr) []byte {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(Key(addr)))
	return mac.Sum(nil)
}

type ctxKey struct{}

// WithAddr stores the resolved client address in ctx.
func WithAddr(ctx context.Context, addr netip.Addr) context.Context {
	return context.WithValue(ctx, ctxKey{}, addr)
}

// FromContext returns the client address stored by the middleware.
func FromContext(ctx context.Context) netip.Addr {
	addr, ok := ctx.Value(ctxKey{}).(netip.Addr)
	if !ok {
		return netip.IPv4Unspecified()
	}
	return addr
}

// Middleware resolves the client IP once per request.
func (res *Resolver) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := WithAddr(r.Context(), res.Resolve(r))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
