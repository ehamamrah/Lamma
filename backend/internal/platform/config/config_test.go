package config

import (
	"strings"
	"testing"
	"time"
)

func TestParseRate(t *testing.T) {
	tests := []struct {
		in      string
		want    Rate
		wantErr bool
	}{
		{"120/m", Rate{120, time.Minute}, false},
		{"5/15m", Rate{5, 15 * time.Minute}, false},
		{"20/10s", Rate{20, 10 * time.Second}, false},
		{"10/h", Rate{10, time.Hour}, false},
		{"0/m", Rate{}, true},
		{"abc/m", Rate{}, true},
		{"10", Rate{}, true},
		{"10/-1s", Rate{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParseRate(tt.in)
			if (err != nil) != tt.wantErr || got != tt.want {
				t.Fatalf("ParseRate(%q) = %v, %v", tt.in, got, err)
			}
		})
	}
}

func baseEnv() map[string]string {
	return map[string]string{
		"DATABASE_URL": "postgres://x", "REDIS_URL": "redis://x",
		"LIVEKIT_URL": "http://lk", "LIVEKIT_PUBLIC_URL": "ws://lk",
		"LIVEKIT_API_KEY": "k", "LIVEKIT_API_SECRET": "lamma-dev-only-livekit-secret-0123456789",
		"ORIGIN_HASH_KEY": "lamma-dev-only-origin-hash-key-0123456789",
		"COOKIE_SECURE":   "false", "APP_ENV": "development",
	}
}

func lookupFrom(env map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) { v, ok := env[k]; return v, ok }
}

func TestLoadDevelopmentDefaults(t *testing.T) {
	cfg, err := LoadFrom(lookupFrom(baseEnv()))
	if err != nil {
		t.Fatalf("dev config should load: %v", err)
	}
	if cfg.Limits.Token != (Rate{6, time.Minute}) || cfg.LiveKit.TokenTTL != 2*time.Minute {
		t.Fatalf("unexpected defaults: %+v %+v", cfg.Limits.Token, cfg.LiveKit.TokenTTL)
	}
}

func TestLoadReportsAllMissingRequired(t *testing.T) {
	_, err := LoadFrom(lookupFrom(map[string]string{}))
	for _, key := range []string{"DATABASE_URL", "REDIS_URL", "LIVEKIT_API_SECRET", "ORIGIN_HASH_KEY"} {
		if err == nil || !strings.Contains(err.Error(), key) {
			t.Fatalf("error should mention %s: %v", key, err)
		}
	}
}

func TestProductionGuards(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(map[string]string)
		wantErr string
	}{
		{"dev secrets rejected", func(map[string]string) {}, "LIVEKIT_API_SECRET"},
		{"insecure cookies rejected", func(e map[string]string) { strongSecrets(e) }, "COOKIE_SECURE"},
		{"trusted proxies required", func(e map[string]string) {
			strongSecrets(e)
			e["COOKIE_SECURE"] = "true"
		}, "TRUSTED_PROXY_CIDRS"},
		{"valid production config", func(e map[string]string) {
			strongSecrets(e)
			e["COOKIE_SECURE"], e["TRUSTED_PROXY_CIDRS"] = "true", "10.0.0.5/32"
			e["PUBLIC_BASE_URL"] = "https://meet.example.com"
			e["LIVEKIT_PUBLIC_URL"] = "wss://rtc.example.com"
		}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := baseEnv()
			env["APP_ENV"] = "production"
			tt.mutate(env)
			_, err := LoadFrom(lookupFrom(env))
			if tt.wantErr == "" && err != nil || tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("got %v, want error containing %q", err, tt.wantErr)
			}
		})
	}
}

func strongSecrets(e map[string]string) {
	e["LIVEKIT_API_SECRET"] = strings.Repeat("s", 40)
	e["ORIGIN_HASH_KEY"] = strings.Repeat("o", 40)
}
