package password

import (
	"context"
	"testing"
	"time"

	"github.com/ehamamrah/lamma/backend/internal/platform/apperr"
)

func TestHashAndVerify(t *testing.T) {
	h := NewHasher(2)
	ctx := context.Background()
	encoded, err := h.Hash(ctx, "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		pw   string
		want bool
	}{
		{"correct horse", true},
		{"correct horsE", false},
		{"", false},
	}
	for _, tt := range tests {
		ok, err := h.Verify(ctx, encoded, tt.pw)
		if err != nil || ok != tt.want {
			t.Fatalf("Verify(%q) = %v, %v; want %v", tt.pw, ok, err, tt.want)
		}
	}
}

func TestHashesAreSalted(t *testing.T) {
	h := NewHasher(1)
	a, _ := h.Hash(context.Background(), "same")
	b, _ := h.Hash(context.Background(), "same")
	if a == b {
		t.Fatal("identical passwords must produce different hashes")
	}
}

func TestVerifyRejectsMalformedOrForeignParams(t *testing.T) {
	h := NewHasher(1)
	for _, enc := range []string{"", "$argon2id$v=19$m=1,t=1,p=1$AAAA$AAAA", "$bcrypt$x"} {
		if _, err := h.Verify(context.Background(), enc, "x"); err == nil {
			t.Fatalf("expected error for %q", enc)
		}
	}
}

func TestGateRejectsWhenSaturated(t *testing.T) {
	h := NewHasher(1)
	if err := h.gate.Acquire(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	defer h.gate.Release(1)
	start := time.Now()
	_, err := h.Hash(context.Background(), "pw")
	if !apperr.IsKind(err, apperr.KindUnavailable) {
		t.Fatalf("want server_busy when gate is full, got %v", err)
	}
	if time.Since(start) > 2*gateWait {
		t.Fatal("gate wait must be bounded")
	}
}
