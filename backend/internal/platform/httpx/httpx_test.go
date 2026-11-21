package httpx

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ehamamrah/lamma/backend/internal/platform/apperr"
)

func TestDecode(t *testing.T) {
	type body struct {
		Name string `json:"name"`
	}
	tests := []struct {
		name, contentType, payload string
		wantErr                    bool
	}{
		{"valid", "application/json", `{"name":"a"}`, false},
		{"charset param ok", "application/json; charset=utf-8", `{"name":"a"}`, false},
		{"wrong content type", "text/plain", `{"name":"a"}`, true},
		{"unknown field", "application/json", `{"name":"a","role":"host"}`, true},
		{"trailing document", "application/json", `{"name":"a"}{"name":"b"}`, true},
		{"too large", "application/json", `{"name":"` + strings.Repeat("a", MaxBodyBytes) + `"}`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.payload))
			r.Header.Set("Content-Type", tt.contentType)
			var b body
			if err := Decode(httptest.NewRecorder(), r, &b); (err != nil) != tt.wantErr {
				t.Fatalf("Decode err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestErrorMapping(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
		retry      string
	}{
		{"not found", apperr.ErrNotFound, 404, "not_found", ""},
		{"rate limited carries retry-after", apperr.ErrRateLimited.WithRetry(1500 * time.Millisecond), 429, "rate_limited", "2"},
		{"unknown errors are hidden", errors.New("pq: secret detail"), 500, "internal", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			Error(rec, httptest.NewRequest(http.MethodGet, "/", nil), tt.err)
			var p Problem
			_ = json.NewDecoder(rec.Body).Decode(&p)
			if rec.Code != tt.wantStatus || p.Code != tt.wantCode || rec.Header().Get("Retry-After") != tt.retry {
				t.Fatalf("got %d %q retry=%q", rec.Code, p.Code, rec.Header().Get("Retry-After"))
			}
			if strings.Contains(p.Title, "secret") {
				t.Fatal("internal error details leaked")
			}
		})
	}
}

func TestSameOrigin(t *testing.T) {
	h := SameOrigin("https://meet.example.com", nil)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	tests := []struct {
		name, method, origin, fetchSite string
		want                            int
	}{
		{"same origin post", http.MethodPost, "https://meet.example.com", "", 204},
		{"cross origin post", http.MethodPost, "https://evil.example", "", 403},
		{"no origin same-origin fetch", http.MethodPost, "", "same-origin", 204},
		{"no origin cross-site fetch", http.MethodPost, "", "cross-site", 403},
		{"no headers at all", http.MethodPost, "", "", 403},
		{"safe method always allowed", http.MethodGet, "https://evil.example", "", 204},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(tt.method, "/", nil)
			if tt.origin != "" {
				r.Header.Set("Origin", tt.origin)
			}
			if tt.fetchSite != "" {
				r.Header.Set("Sec-Fetch-Site", tt.fetchSite)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, r)
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d", rec.Code, tt.want)
			}
		})
	}
}

func TestRequestIDRejectsMalformedInput(t *testing.T) {
	var seen string
	h := RequestID(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { seen = w.Header().Get("X-Request-ID") }))
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-Request-ID", "bad id\nwith newline")
	h.ServeHTTP(httptest.NewRecorder(), r)
	if !validRequestID.MatchString(seen) || strings.Contains(seen, "\n") {
		t.Fatalf("malformed request id propagated: %q", seen)
	}
}
