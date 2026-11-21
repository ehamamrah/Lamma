// Package httpx contains HTTP plumbing shared by all modules: JSON I/O,
// RFC 9457 problem responses and middleware.
package httpx

import (
	"encoding/json"
	"io"
	"log/slog"
	"math"
	"mime"
	"net/http"
	"strconv"

	"github.com/ehamamrah/lamma/backend/internal/platform/apperr"
)

// MaxBodyBytes bounds every request body (PLAN.md §10).
const MaxBodyBytes = 16 << 10

// Problem is an RFC 9457 problem details body.
type Problem struct {
	Type   string `json:"type"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Code   string `json:"code"`
}

var kindStatus = map[apperr.Kind]int{
	apperr.KindInvalid:      http.StatusBadRequest,
	apperr.KindUnauthorized: http.StatusUnauthorized,
	apperr.KindForbidden:    http.StatusForbidden,
	apperr.KindNotFound:     http.StatusNotFound,
	apperr.KindConflict:     http.StatusConflict,
	apperr.KindRateLimited:  http.StatusTooManyRequests,
	apperr.KindUnavailable:  http.StatusServiceUnavailable,
}

// JSON writes v with the given status.
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

// Error writes err as a problem response. Unknown errors become a generic 500
// and are logged with context; their details never reach the client.
func Error(w http.ResponseWriter, r *http.Request, err error) {
	appErr, ok := apperr.As(err)
	if !ok {
		slog.ErrorContext(r.Context(), "unhandled error", slog.Any("error", err))
		appErr = apperr.New(apperr.KindInternal, "internal", "Something went wrong.")
	}
	status, known := kindStatus[appErr.Kind]
	if !known {
		status = http.StatusInternalServerError
	}
	if appErr.RetryAfter > 0 {
		secs := int(math.Ceil(appErr.RetryAfter.Seconds()))
		w.Header().Set("Retry-After", strconv.Itoa(secs))
	}
	writeProblem(w, status, appErr)
}

func writeProblem(w http.ResponseWriter, status int, e *apperr.Error) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Problem{
		Type: "https://lamma.dev/problems/" + e.Code, Title: e.Message, Status: status, Code: e.Code,
	})
}

var errBadJSON = apperr.New(apperr.KindInvalid, "invalid_body", "Request body is not valid JSON.")

// Decode strictly decodes a JSON body: size-limited, JSON content type only,
// unknown fields rejected, single document.
func Decode(w http.ResponseWriter, r *http.Request, dst any) error {
	mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mt != "application/json" {
		return apperr.New(apperr.KindInvalid, "unsupported_media_type", "Use Content-Type: application/json.")
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, MaxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return errBadJSON
	}
	if dec.Decode(&struct{}{}) != io.EOF {
		return errBadJSON
	}
	return nil
}
