package config

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const minSecretLen = 32

// devMarker appears in every dev-only default shipped in compose.yaml.
const devMarker = "dev-only"

func validate(c Config) error {
	errs := validateCommon(c)
	if c.IsProduction() {
		errs = append(errs, validateProduction(c)...)
	}
	return errors.Join(errs...)
}

func validateCommon(c Config) []error {
	var errs []error
	if c.Env != "development" && c.Env != "production" {
		errs = append(errs, fmt.Errorf("APP_ENV: must be development or production"))
	}
	if c.Meeting.DefaultTTL > c.Meeting.MaxTTL {
		errs = append(errs, fmt.Errorf("MEETING_DEFAULT_TTL: exceeds MEETING_MAX_TTL"))
	}
	if c.Meeting.MaxParticipants < 2 || c.Meeting.MaxParticipants > 500 {
		errs = append(errs, fmt.Errorf("MEETING_MAX_PARTICIPANTS: must be 2..500"))
	}
	if c.LiveKit.TokenTTL < 30*time.Second || c.LiveKit.TokenTTL > 5*time.Minute {
		errs = append(errs, fmt.Errorf("LIVEKIT_TOKEN_TTL: must be between 30s and 5m"))
	}
	if c.Argon2MaxConcurrent < 1 || c.DBMaxConns < 2 {
		errs = append(errs, fmt.Errorf("ARGON2_MAX_CONCURRENT >= 1 and DB_MAX_CONNS >= 2 required"))
	}
	return errs
}

// validateProduction implements the production guards from PLAN.md §11.
func validateProduction(c Config) []error {
	var errs []error
	errs = appendIfWeak(errs, "LIVEKIT_API_SECRET", c.LiveKit.APISecret)
	errs = appendIfWeak(errs, "ORIGIN_HASH_KEY", string(c.OriginHashKey))
	if !c.CookieSecure {
		errs = append(errs, fmt.Errorf("COOKIE_SECURE: must be true in production"))
	}
	if len(c.TrustedProxies) == 0 {
		errs = append(errs, fmt.Errorf("TRUSTED_PROXY_CIDRS: required in production"))
	}
	if !strings.HasPrefix(c.PublicBaseURL, "https://") || !strings.HasPrefix(c.LiveKit.PublicURL, "wss://") {
		errs = append(errs, fmt.Errorf("PUBLIC_BASE_URL/LIVEKIT_PUBLIC_URL: must use https/wss in production"))
	}
	for _, o := range c.CORSOrigins {
		if !strings.HasPrefix(o, "https://") {
			errs = append(errs, fmt.Errorf("CORS_ALLOWED_ORIGINS: %q must be https in production", o))
		}
	}
	return errs
}

func appendIfWeak(errs []error, key, secret string) []error {
	if len(secret) < minSecretLen || strings.Contains(secret, devMarker) {
		return append(errs, fmt.Errorf("%s: must be >= %d bytes and not a dev value", key, minSecretLen))
	}
	return errs
}
