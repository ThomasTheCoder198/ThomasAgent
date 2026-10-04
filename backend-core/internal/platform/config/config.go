package config

import (
	"errors"
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/logging"
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
	MaxBodyBytes         int64         `env:"MAX_BODY_BYTES" envDefault:"1048576"`
	RequestTimeout       time.Duration `env:"REQUEST_TIMEOUT" envDefault:"30s"`
	EventStreamHeartbeat time.Duration `env:"EVENT_STREAM_HEARTBEAT" envDefault:"15s"`
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
	return errs
}

type VaultConfig struct {
	MasterKey string `env:"MASTER_KEY,required,notEmpty"`
	KeyID     string `env:"KEY_ID" envDefault:"v1"`
}

type Config struct {
	Vault             VaultConfig       `envPrefix:"VAULT_"`
	HTTP              HTTPConfig        `envPrefix:"HTTP_"`
	Environment       string            `env:"ENV" envDefault:"dev"`
	ServiceName       string            `env:"SERVICE_NAME" envDefault:"thomas-core"`
	HTTPAddr          string            `env:"HTTP_ADDR" envDefault:":8080"`
	ShutdownTimeout   time.Duration     `env:"SHUTDOWN_TIMEOUT" envDefault:"15s"`
	ReadHeaderTimeout time.Duration     `env:"READ_HEADER_TIMEOUT" envDefault:"10s"`
	DatabaseURL       string            `env:"DATABASE_URL,required,notEmpty"`
	RedisURL          string            `env:"REDIS_URL,required,notEmpty"`
	OTLPEndpoint      string            `env:"OTLP_ENDPOINT"`
	LogLevel          string            `env:"LOG_LEVEL" envDefault:"info"`
	Retry             RetryConfig       `envPrefix:"RETRY_"`
	Breaker           BreakerConfig     `envPrefix:"BREAKER_"`
	Stream            ConsumerConfig    `envPrefix:"STREAM_"`
	OutboxRelay       OutboxRelayConfig `envPrefix:"RELAY_"`
}

const envPrefix = "CORE_"

func Load() (Config, error) {
	cfg, err := env.ParseAsWithOptions[Config](env.Options{Prefix: envPrefix})
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
	if c.Breaker.FailureThreshold < 1 {
		errs = append(errs, errors.New("CORE_BREAKER_FAILURE_THRESHOLD must be >= 1"))
	}
	if c.Breaker.HalfOpenMaxCalls < 1 {
		errs = append(errs, errors.New("CORE_BREAKER_HALF_OPEN_MAX_CALLS must be >= 1"))
	}
	errs = append(errs, c.HTTP.validate()...)
	return errors.Join(errs...)
}
