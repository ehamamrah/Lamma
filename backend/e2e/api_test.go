//go:build e2e

// Package e2e exercises the running stack's API end to end:
//
//	docker compose up -d migrate api
//	GO_DOCKER_ARGS="--network lamma_lamma -e LAMMA_API=http://api:8000" scripts/go.sh go test -tags=e2e ./e2e/...
package e2e

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"os"
	"testing"
)

const origin = "http://localhost:8080" // PUBLIC_BASE_URL of the dev stack

type client struct {
	t    *testing.T
	base string
	http *http.Client
}

func newClient(t *testing.T) *client {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	base := os.Getenv("LAMMA_API")
	if base == "" {
		base = "http://localhost:8000"
	}
	return &client{t: t, base: base, http: &http.Client{Jar: jar}}
}

func (c *client) do(method, path string, body any) (int, map[string]any) {
	c.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.base+path, rd)
	req.Header.Set("Origin", origin)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	out := map[string]any{}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func (c *client) expect(want int, method, path string, body any) map[string]any {
	c.t.Helper()
	got, out := c.do(method, path, body)
	if got != want {
		c.t.Fatalf("%s %s = %d, want %d: %v", method, path, got, want, out)
	}
	return out
}

func TestMeetingLifecycle(t *testing.T) {
	host, guest, outsider := newClient(t), newClient(t), newClient(t)

	created := host.expect(201, "POST", "/api/v1/meetings", map[string]any{
		"displayName": "Host", "title": "Standup", "waitingRoom": true, "expiresInMinutes": 60,
	})
	code := created["meeting"].(map[string]any)["code"].(string)
	m := "/api/v1/meetings/" + code

	outsider.expect(200, "GET", m, nil)
	outsider.expect(404, "GET", "/api/v1/meetings/zzzz-zzzz-zzzz", nil)
	host.expect(200, "POST", m+"/token", nil)

	joined := guest.expect(202, "POST", m+"/join", map[string]any{"displayName": "Guest"})
	guestID := joined["participant"].(map[string]any)["id"].(string)
	guest.expect(403, "POST", m+"/token", nil) // waiting guests get no media
	guest.expect(403, "GET", m+"/participants", nil)

	waiting := host.expect(200, "GET", m+"/participants", nil)["participants"].([]any)
	if len(waiting) != 1 {
		t.Fatalf("waiting list = %v", waiting)
	}
	host.expect(204, "POST", m+"/participants/"+guestID+"/admit", nil)
	guest.expect(200, "POST", m+"/token", nil)

	// Privilege escalation attempts by a guest must fail server-side.
	guest.expect(403, "POST", m+"/end", nil)
	guest.expect(403, "PATCH", m+"/settings", map[string]any{"locked": true})
	guest.expect(400, "POST", m+"/join", map[string]any{"displayName": "G", "role": "host"})

	guest.expect(201, "POST", m+"/chat", map[string]any{"text": "hello"})
	if msgs := guest.expect(200, "GET", m+"/chat", nil)["messages"].([]any); len(msgs) != 1 {
		t.Fatalf("chat history = %v", msgs)
	}

	host.expect(200, "PATCH", m+"/settings", map[string]any{"locked": true})
	outsider.expect(403, "POST", m+"/join", map[string]any{"displayName": "Late"})

	host.expect(204, "POST", m+"/participants/"+guestID+"/remove", nil)
	guest.expect(403, "POST", m+"/token", nil)
	guest.expect(403, "POST", m+"/join", map[string]any{"displayName": "Guest"})

	host.expect(204, "POST", m+"/end", nil)
	outsider.expect(404, "GET", m, nil)
	host.expect(404, "POST", m+"/token", nil)
}

func TestPasswordProtectedMeeting(t *testing.T) {
	host, guest := newClient(t), newClient(t)
	created := host.expect(201, "POST", "/api/v1/meetings", map[string]any{
		"displayName": "Host", "password": "s3cret-pass",
	})
	m := "/api/v1/meetings/" + created["meeting"].(map[string]any)["code"].(string)
	if !guest.expect(200, "GET", m, nil)["requiresPassword"].(bool) {
		t.Fatal("public info must report the password requirement")
	}
	guest.expect(401, "POST", m+"/join", map[string]any{"displayName": "G"})
	guest.expect(401, "POST", m+"/join", map[string]any{"displayName": "G", "password": "wrong-pass"})
	guest.expect(200, "POST", m+"/join", map[string]any{"displayName": "G", "password": "s3cret-pass"})
	guest.expect(200, "POST", m+"/join", map[string]any{"displayName": "G"}) // session resume: no password
}

func TestCrossSiteRequestsRejected(t *testing.T) {
	c := newClient(t)
	req, _ := http.NewRequest("POST", c.base+"/api/v1/meetings", bytes.NewReader([]byte(`{"displayName":"x"}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://evil.example")
	resp, err := c.http.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-site create = %d, want 403", resp.StatusCode)
	}
}
