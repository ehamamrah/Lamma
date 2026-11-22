// Package config loads and validates all runtime configuration from the
// environment. It is the only package that reads environment variables.
package config

import (
	"errors"
	"log/slog"
	"net/netip"
	"runtime"
	"time"
)

// Config is the complete, validated application configuration.
type Config struct {
	Env                 string
	LogLevel            slog.Level
	HTTPAddr            string
	MetricsAddr         string
	PublicBaseURL       string
	DatabaseURL         string
	MigrateURL          string
	DBMaxConns          int
	RedisURL            string
	CookieSecure        bool
	TrustedProxies      []netip.Prefix
	CORSOrigins         []string
	OriginHashKey       []byte
	Argon2MaxConcurrent int
	OTLPEndpoint        string
	LiveKit             LiveKit
	Meeting             Meeting
	Limits              Limits
	Breaker             Breaker
}

// LiveKit holds LiveKit server API and token settings.
type LiveKit struct {
	URL         string
	PublicURL   string
	APIKey      string
	APISecret   string
	TokenTTL    time.Duration
	CallTimeout time.Duration
}

// Meeting holds meeting lifecycle defaults.
type Meeting struct {
	DefaultTTL time.Duration
	MaxTTL     time.Duration
	Retention  time.Duration
}

// Limits holds every rate-limit policy (PLAN.md §10).
type Limits struct {
	General, Create, Lookup, JoinIP, JoinMeeting, Password Rate
	Token, Chat, ChatMeeting, Moderation                   Rate
}

// Breaker holds circuit-breaker settings shared by the LiveKit breakers.
type Breaker struct {
	FailureThreshold uint32
	OpenTimeout      time.Duration
}

// IsProduction reports whether production guards apply.
func (c Config) IsProduction() bool { return c.Env == "production" }

// LogValue keeps secrets out of logs when the config is logged.
func (c Config) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("env", c.Env),
		slog.String("http_addr", c.HTTPAddr),
		slog.String("public_base_url", c.PublicBaseURL),
		slog.Bool("cookie_secure", c.CookieSecure),
		slog.Int("db_max_conns", c.DBMaxConns),
		slog.Bool("tracing", c.OTLPEndpoint != ""),
	)
}

// Load reads the configuration from the process environment.
func Load() (Config, error) { return LoadFrom(nil) }

// LoadFrom reads configuration using lookup (nil = os.LookupEnv).
func LoadFrom(lookup func(string) (string, bool)) (Config, error) {
	r := newReader(lookup)
	c := loadApp(r)
	c.LiveKit = loadLiveKit(r)
	c.Meeting = loadMeeting(r)
	c.Limits = loadLimits(r)
	c.Breaker = loadBreaker(r)
	if err := errors.Join(r.errs...); err != nil {
		return Config{}, err
	}
	return c, validate(c)
}

func loadApp(r *reader) Config {
	return Config{
		Env:                 r.str("APP_ENV", "production"), // fail safe: guards on by default
		LogLevel:            parseLevel(r, r.str("LOG_LEVEL", "info")),
		HTTPAddr:            r.str("HTTP_ADDR", ":8000"),
		MetricsAddr:         r.str("METRICS_ADDR", ":9090"),
		PublicBaseURL:       r.str("PUBLIC_BASE_URL", "http://localhost:8080"),
		DatabaseURL:         r.required("DATABASE_URL"),
		MigrateURL:          r.str("DATABASE_MIGRATE_URL", ""),
		DBMaxConns:          r.integer("DB_MAX_CONNS", 20),
		RedisURL:            r.required("REDIS_URL"),
		CookieSecure:        r.boolean("COOKIE_SECURE", true),
		TrustedProxies:      r.prefixes("TRUSTED_PROXY_CIDRS", ""),
		CORSOrigins:         r.list("CORS_ALLOWED_ORIGINS"),
		OriginHashKey:       []byte(r.required("ORIGIN_HASH_KEY")),
		Argon2MaxConcurrent: r.integer("ARGON2_MAX_CONCURRENT", runtime.NumCPU()),
		OTLPEndpoint:        r.str("OTEL_EXPORTER_OTLP_ENDPOINT", ""),
	}
}

func loadLiveKit(r *reader) LiveKit {
	return LiveKit{
		URL:         r.required("LIVEKIT_URL"),
		PublicURL:   r.required("LIVEKIT_PUBLIC_URL"),
		APIKey:      r.required("LIVEKIT_API_KEY"),
		APISecret:   r.required("LIVEKIT_API_SECRET"),
		TokenTTL:    r.duration("LIVEKIT_TOKEN_TTL", 2*time.Minute),
		CallTimeout: r.duration("LIVEKIT_CALL_TIMEOUT", 3*time.Second),
	}
}

func loadMeeting(r *reader) Meeting {
	return Meeting{
		DefaultTTL: r.duration("MEETING_DEFAULT_TTL", 24*time.Hour),
		MaxTTL:     r.duration("MEETING_MAX_TTL", 168*time.Hour),
		Retention:  r.duration("MEETING_RETENTION", 168*time.Hour),
	}
}

func loadLimits(r *reader) Limits {
	return Limits{
		General:     r.rate("RATE_LIMIT_GENERAL", "120/m"),
		Create:      r.rate("RATE_LIMIT_CREATE", "10/h"),
		Lookup:      r.rate("RATE_LIMIT_LOOKUP", "30/m"),
		JoinIP:      r.rate("RATE_LIMIT_JOIN_IP", "10/m"),
		JoinMeeting: r.rate("RATE_LIMIT_JOIN_MEETING", "5/m"),
		Password:    r.rate("RATE_LIMIT_PASSWORD", "5/15m"),
		Token:       r.rate("RATE_LIMIT_TOKEN", "6/m"),
		Chat:        r.rate("RATE_LIMIT_CHAT", "20/10s"),
		ChatMeeting: r.rate("RATE_LIMIT_CHAT_MEETING", "10/s"),
		Moderation:  r.rate("RATE_LIMIT_MODERATION", "60/m"),
	}
}

func loadBreaker(r *reader) Breaker {
	return Breaker{
		FailureThreshold: uint32(min(1000, max(1, r.integer("BREAKER_FAILURE_THRESHOLD", 5)))), //nolint:gosec // clamped
		OpenTimeout:      r.duration("BREAKER_OPEN_TIMEOUT", 30*time.Second),
	}
}

func parseLevel(r *reader, s string) slog.Level {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(s)); err != nil {
		r.fail("LOG_LEVEL", err)
	}
	return lvl
}
