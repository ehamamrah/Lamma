// Package password hashes and verifies meeting passwords with argon2id behind
// a concurrency gate, so password traffic cannot exhaust memory (PLAN.md §11).
package password

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"
	"golang.org/x/sync/semaphore"

	"github.com/ehamamrah/lamma/backend/internal/platform/apperr"
)

// OWASP-recommended argon2id parameters: 19 MiB, 2 iterations, 1 lane.
const (
	memoryKiB = 19 * 1024
	time2     = 2
	threads   = 1
	keyLen    = 32
	saltLen   = 16
)

// gateWait bounds how long a request waits for a hashing slot before 503.
const gateWait = time.Second

var errMalformed = errors.New("malformed password hash")

// Hasher is safe for concurrent use.
type Hasher struct {
	gate *semaphore.Weighted
}

// NewHasher allows at most maxConcurrent argon2 computations at once.
func NewHasher(maxConcurrent int) *Hasher {
	return &Hasher{gate: semaphore.NewWeighted(int64(max(1, maxConcurrent)))}
}

// Hash returns a PHC-formatted argon2id hash of pw.
func (h *Hasher) Hash(ctx context.Context, pw string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key, err := h.derive(ctx, pw, salt)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, memoryKiB, time2, threads,
		b64(salt), b64(key)), nil
}

// Verify reports whether pw matches encoded, in constant time.
func (h *Hasher) Verify(ctx context.Context, encoded, pw string) (bool, error) {
	salt, want, err := parse(encoded)
	if err != nil {
		return false, err
	}
	got, err := h.derive(ctx, pw, salt)
	if err != nil {
		return false, err
	}
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

func (h *Hasher) derive(ctx context.Context, pw string, salt []byte) ([]byte, error) {
	waitCtx, cancel := context.WithTimeout(ctx, gateWait)
	defer cancel()
	if err := h.gate.Acquire(waitCtx, 1); err != nil {
		return nil, apperr.ErrBusy.WithRetry(2 * time.Second)
	}
	defer h.gate.Release(1)
	return argon2.IDKey([]byte(pw), salt, time2, memoryKiB, threads, keyLen), nil
}

func parse(encoded string) (salt, key []byte, err error) {
	parts := strings.Split(encoded, "$")
	want := fmt.Sprintf("m=%d,t=%d,p=%d", memoryKiB, time2, threads)
	if len(parts) != 6 || parts[1] != "argon2id" || parts[3] != want {
		return nil, nil, errMalformed
	}
	if salt, err = unb64(parts[4]); err != nil {
		return nil, nil, errMalformed
	}
	if key, err = unb64(parts[5]); err != nil {
		return nil, nil, errMalformed
	}
	return salt, key, nil
}

func b64(b []byte) string            { return base64.RawStdEncoding.EncodeToString(b) }
func unb64(s string) ([]byte, error) { return base64.RawStdEncoding.DecodeString(s) }
