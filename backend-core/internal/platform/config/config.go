package config

import (
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/logging"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/outbound"
)

type RetryConfig struct {
	MaxAttempts int           `env:"MAX_ATTEMPTS" envDefault:"4"`
	BaseDelay   time.Duration `env:"BASE_DELAY" envDefault:"200ms"`
	MaxDelay    time.Duration `env:"MAX_DELAY" envDefault:"10s"`
}

type BreakerConfig struct {
	FailureThreshold uint32        `env:"FAILURE_THRESHOLD" envDefault:"5"`
	OpenTimeout      time.Duration `env:"OPEN_TIMEOUT" envDefault:"30s"`
	HalfOpenMaxCalls uint32        `env:"HALF_OPEN_MAX_CALLS" envDefault:"1"`
}

type ConsumerConfig struct {
	MaxDeliveries     int           `env:"MAX_DELIVERIES" envDefault:"4"`
	VisibilityTimeout time.Duration `env:"VISIBILITY_TIMEOUT" envDefault:"30s"`
	BlockTimeout      time.Duration `env:"BLOCK_TIMEOUT" envDefault:"5s"`
	BatchSize         int64         `env:"BATCH_SIZE" envDefault:"16"`
}

type OutboxRelayConfig struct {
	BatchSize    int           `env:"BATCH_SIZE" envDefault:"100"`
	PollInterval time.Duration `env:"POLL_INTERVAL" envDefault:"500ms"`
}

type HTTPConfig struct {
	MaxBodyBytes            int64         `env:"MAX_BODY_BYTES" envDefault:"1048576"`
	RequestTimeout          time.Duration `env:"REQUEST_TIMEOUT" envDefault:"30s"`
	EventStreamHeartbeat    time.Duration `env:"EVENT_STREAM_HEARTBEAT" envDefault:"15s"`
	EventStreamWriteTimeout time.Duration `env:"EVENT_STREAM_WRITE_TIMEOUT" envDefault:"10s"`
}

func (c HTTPConfig) validate() []error {
	var errs []error
	if c.MaxBodyBytes < 1 {
		errs = append(errs, errors.New("CORE_HTTP_MAX_BODY_BYTES must be >= 1"))
	}
	if c.RequestTimeout <= 0 {
		errs = append(errs, errors.New("CORE_HTTP_REQUEST_TIMEOUT must be > 0"))
	}
	if c.EventStreamHeartbeat <= 0 {
		errs = append(errs, errors.New("CORE_HTTP_EVENT_STREAM_HEARTBEAT must be > 0"))
	}
	if c.EventStreamWriteTimeout <= 0 {
		errs = append(errs, errors.New("CORE_HTTP_EVENT_STREAM_WRITE_TIMEOUT must be > 0"))
	}
	return errs
}

type VaultConfig struct {
	MasterKey string `env:"MASTER_KEY,required,notEmpty"`
	KeyID     string `env:"KEY_ID" envDefault:"v1"`
}

type AuthConfig struct {
	OwnerEmail            string             `env:"OWNER_EMAIL"`
	OwnerPassword         string             `env:"OWNER_PASSWORD"`
	SessionTTL            time.Duration      `env:"SESSION_TTL" envDefault:"720h"`
	CookieSecure          bool               `env:"COOKIE_SECURE" envDefault:"false"`
	LoginMaxAttempts      int                `env:"LOGIN_MAX_ATTEMPTS" envDefault:"10"`
	LoginEmailMaxAttempts int                `env:"LOGIN_EMAIL_MAX_ATTEMPTS" envDefault:"20"`
	LoginIPMaxAttempts    int                `env:"LOGIN_IP_MAX_ATTEMPTS" envDefault:"100"`
	LoginWindow           time.Duration      `env:"LOGIN_WINDOW" envDefault:"15m"`
	MinPasswordLength     int                `env:"MIN_PASSWORD_LENGTH" envDefault:"12"`
	TrustedProxyHeader    string             `env:"TRUSTED_PROXY_HEADER"`
	TrustedProxyCIDRs     []string           `env:"TRUSTED_PROXY_CIDRS" envSeparator:","`
	SessionPurgeInterval  time.Duration      `env:"SESSION_PURGE_INTERVAL" envDefault:"1h"`
	SessionTouchInterval  time.Duration      `env:"SESSION_TOUCH_INTERVAL" envDefault:"5m"`
	PasswordHash          PasswordHashConfig `envPrefix:"PASSWORD_HASH_"`
}

type PasswordHashConfig struct {
	Iterations     uint32 `env:"ITERATIONS" envDefault:"2"`
	MemoryKiB      uint32 `env:"MEMORY_KIB" envDefault:"65536"`
	MaxConcurrency int    `env:"MAX_CONCURRENCY" envDefault:"4"`
	Parallelism    uint8  `env:"PARALLELISM" envDefault:"1"`
	SaltLength     uint32 `env:"SALT_LENGTH" envDefault:"16"`
	KeyLength      uint32 `env:"KEY_LENGTH" envDefault:"32"`
}

func (c AuthConfig) validate() []error {
	errs := c.validateSessionAndLogin()
	errs = append(errs, c.PasswordHash.validate()...)
	for _, cidr := range c.TrustedProxyCIDRs {
		if _, err := netip.ParsePrefix(cidr); err != nil {
			errs = append(errs, errors.New("CORE_AUTH_TRUSTED_PROXY_CIDRS must contain valid CIDRs"))
		}
	}
	return errs
}

func (c AuthConfig) validateSessionAndLogin() []error {
	var errs []error
	if c.SessionTouchInterval <= 0 {
		errs = append(errs, errors.New("CORE_AUTH_SESSION_TOUCH_INTERVAL must be > 0"))
	}
	if c.LoginEmailMaxAttempts < 1 {
		errs = append(errs, errors.New("CORE_AUTH_LOGIN_EMAIL_MAX_ATTEMPTS must be >= 1"))
	}
	if c.LoginIPMaxAttempts < 1 {
		errs = append(errs, errors.New("CORE_AUTH_LOGIN_IP_MAX_ATTEMPTS must be >= 1"))
	}
	if c.SessionTTL <= 0 {
		errs = append(errs, errors.New("CORE_AUTH_SESSION_TTL must be > 0"))
	}
	if c.SessionPurgeInterval <= 0 {
		errs = append(errs, errors.New("CORE_AUTH_SESSION_PURGE_INTERVAL must be > 0"))
	}
	if c.MinPasswordLength < 1 {
		errs = append(errs, errors.New("CORE_AUTH_MIN_PASSWORD_LENGTH must be >= 1"))
	}
	if c.LoginWindow <= 0 {
		errs = append(errs, errors.New("CORE_AUTH_LOGIN_WINDOW must be > 0"))
	}
	if c.LoginMaxAttempts < 1 {
		errs = append(errs, errors.New("CORE_AUTH_LOGIN_MAX_ATTEMPTS must be >= 1"))
	}
	return errs
}

func (c PasswordHashConfig) validate() []error {
	var errs []error
	if c.Parallelism < 1 {
		errs = append(errs, errors.New("CORE_AUTH_PASSWORD_HASH_PARALLELISM must be >= 1"))
	}
	if c.SaltLength < MinimumArgonSaltLength {
		errs = append(errs, errors.New("CORE_AUTH_PASSWORD_HASH_SALT_LENGTH must be >= 8"))
	}
	if c.KeyLength < MinimumArgonKeyLength {
		errs = append(errs, errors.New("CORE_AUTH_PASSWORD_HASH_KEY_LENGTH must be >= 16"))
	}
	if c.Iterations < MinimumArgonIterations {
		errs = append(errs, errors.New("CORE_AUTH_PASSWORD_HASH_ITERATIONS must be >= 2"))
	}
	if c.MemoryKiB != ArgonMemoryKiB {
		errs = append(errs, errors.New("CORE_AUTH_PASSWORD_HASH_MEMORY_KIB must equal 65536"))
	}
	if c.MaxConcurrency < 1 {
		errs = append(errs, errors.New("CORE_AUTH_PASSWORD_HASH_MAX_CONCURRENCY must be >= 1"))
	}
	return errs
}

type Config struct {
	ProviderMaxRemoteModels  int               `env:"PROVIDER_MAX_REMOTE_MODELS" envDefault:"1000"`
	ProviderMaxBreakers      int               `env:"PROVIDER_MAX_BREAKERS" envDefault:"256"`
	ProviderHTTPTimeout      time.Duration     `env:"PROVIDER_HTTP_TIMEOUT" envDefault:"20s"`
	ProviderBreaker          BreakerConfig     `envPrefix:"PROVIDER_BREAKER_"`
	ProviderMaxResponseBytes int64             `env:"PROVIDER_MAX_RESPONSE_BYTES"`
	ProviderPrivateAllowlist []string          `env:"PROVIDER_PRIVATE_ALLOWLIST" envSeparator:","`
	Auth                     AuthConfig        `envPrefix:"AUTH_"`
	ServiceToken             string            `env:"SERVICE_TOKEN,required,notEmpty"`
	Vault                    VaultConfig       `envPrefix:"VAULT_"`
	HTTP                     HTTPConfig        `envPrefix:"HTTP_"`
	Environment              string            `env:"ENV" envDefault:"dev"`
	ServiceName              string            `env:"SERVICE_NAME" envDefault:"thomas-core"`
	HTTPAddr                 string            `env:"HTTP_ADDR" envDefault:":8080"`
	ShutdownTimeout          time.Duration     `env:"SHUTDOWN_TIMEOUT" envDefault:"15s"`
	ReadHeaderTimeout        time.Duration     `env:"READ_HEADER_TIMEOUT" envDefault:"10s"`
	DatabaseURL              string            `env:"DATABASE_URL,required,notEmpty"`
	RedisURL                 string            `env:"REDIS_URL,required,notEmpty"`
	OTLPEndpoint             string            `env:"OTLP_ENDPOINT"`
	LogLevel                 string            `env:"LOG_LEVEL" envDefault:"info"`
	Retry                    RetryConfig       `envPrefix:"RETRY_"`
	Stream                   ConsumerConfig    `envPrefix:"STREAM_"`
	OutboxRelay              OutboxRelayConfig `envPrefix:"RELAY_"`
}

const envPrefix = "CORE_"

const DefaultProviderMaxResponseBytes int64 = 8388608

const (
	MinimumArgonIterations         uint32 = 2
	ArgonMemoryKiB                 uint32 = 65536
	DefaultArgonConcurrency               = 4
	MinimumServiceTokenLength             = 32
	DefaultLoginEmailMaxAttempts          = 20
	DefaultLoginIPMaxAttempts             = 100
	DefaultArgonParallelism        uint8  = 1
	DefaultArgonSaltLength         uint32 = 16
	DefaultArgonKeyLength          uint32 = 32
	MinimumArgonSaltLength         uint32 = 8
	MinimumArgonKeyLength          uint32 = 16
	DefaultSessionTouchInterval           = 5 * time.Minute
	DefaultProviderMaxRemoteModels        = 1000
	DefaultProviderMaxBreakers            = 256
)

func Load() (Config, error) {
	cfg := Config{ProviderMaxResponseBytes: DefaultProviderMaxResponseBytes}
	err := env.ParseWithOptions(&cfg, env.Options{Prefix: envPrefix})
	if err != nil {
		return Config{}, fmt.Errorf("load config: %w", err)
	}
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) validate() error {
	var errs []error
	if _, err := logging.ParseLevel(c.LogLevel); err != nil {
		errs = append(errs, fmt.Errorf("CORE_LOG_LEVEL: %w", err))
	}
	if c.Retry.MaxDelay < c.Retry.BaseDelay {
		errs = append(errs, errors.New("CORE_RETRY_MAX_DELAY must be >= CORE_RETRY_BASE_DELAY"))
	}
	if c.Retry.MaxAttempts < 1 || c.Stream.MaxDeliveries < 1 {
		errs = append(errs, errors.New("CORE_RETRY_MAX_ATTEMPTS and CORE_STREAM_MAX_DELIVERIES must be >= 1"))
	}
	errs = append(errs, c.HTTP.validate()...)
	errs = append(errs, c.Auth.validate()...)
	errs = append(errs, c.validateServiceToken()...)
	errs = append(errs, c.validateProvider()...)
	return errors.Join(errs...)
}

func (c Config) validateServiceToken() []error {
	var errs []error
	if len(c.ServiceToken) < MinimumServiceTokenLength {
		errs = append(errs, errors.New("CORE_SERVICE_TOKEN must be at least 32 characters"))
	}
	if !strings.EqualFold(c.Environment, "dev") && strings.HasPrefix(strings.ToLower(c.ServiceToken), "change-me") {
		errs = append(errs, errors.New("CORE_SERVICE_TOKEN must not use a change-me value outside dev"))
	}
	return errs
}

func (c Config) validateProvider() []error {
	var errs []error
	if c.ProviderMaxRemoteModels < 1 {
		errs = append(errs, errors.New("CORE_PROVIDER_MAX_REMOTE_MODELS must be >= 1"))
	}
	if c.ProviderMaxBreakers < 1 {
		errs = append(errs, errors.New("CORE_PROVIDER_MAX_BREAKERS must be >= 1"))
	}
	if c.ProviderHTTPTimeout <= 0 {
		errs = append(errs, errors.New("CORE_PROVIDER_HTTP_TIMEOUT must be > 0"))
	}
	if c.ProviderMaxResponseBytes < 1 {
		errs = append(errs, errors.New("CORE_PROVIDER_MAX_RESPONSE_BYTES must be >= 1"))
	}
	if c.ProviderBreaker.FailureThreshold < 1 {
		errs = append(errs, errors.New("CORE_PROVIDER_BREAKER_FAILURE_THRESHOLD must be >= 1"))
	}
	if c.ProviderBreaker.HalfOpenMaxCalls < 1 {
		errs = append(errs, errors.New("CORE_PROVIDER_BREAKER_HALF_OPEN_MAX_CALLS must be >= 1"))
	}
	if c.ProviderBreaker.OpenTimeout <= 0 {
		errs = append(errs, errors.New("CORE_PROVIDER_BREAKER_OPEN_TIMEOUT must be > 0"))
	}
	if err := outbound.ValidatePrivateAllowlist(c.ProviderPrivateAllowlist); err != nil {
		errs = append(errs, fmt.Errorf("CORE_PROVIDER_PRIVATE_ALLOWLIST: %w", err))
	}
	return errs
}
