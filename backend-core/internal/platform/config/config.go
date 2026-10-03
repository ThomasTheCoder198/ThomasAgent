package config

import (
	"errors"
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
)

const (
	logLevelDebug = "debug"
	logLevelInfo  = "info"
	logLevelWarn  = "warn"
	logLevelError = "error"
)

type RetryConfig struct {
	MaxAttempts int           `env:"MAX_ATTEMPTS" envDefault:"4"`
	BaseDelay   time.Duration `env:"BASE_DELAY" envDefault:"200ms"`
	MaxDelay    time.Duration `env:"MAX_DELAY" envDefault:"10s"`
}

type StreamConfig struct {
	MaxDeliveries     int           `env:"MAX_DELIVERIES" envDefault:"5"`
	VisibilityTimeout time.Duration `env:"VISIBILITY_TIMEOUT" envDefault:"30s"`
	BlockTimeout      time.Duration `env:"BLOCK_TIMEOUT" envDefault:"5s"`
	BatchSize         int64         `env:"BATCH_SIZE" envDefault:"16"`
}

type RelayConfig struct {
	BatchSize    int           `env:"BATCH_SIZE" envDefault:"100"`
	PollInterval time.Duration `env:"POLL_INTERVAL" envDefault:"500ms"`
}

type Config struct {
	Env               string        `env:"ENV" envDefault:"dev"`
	ServiceName       string        `env:"SERVICE_NAME" envDefault:"thomas-core"`
	HTTPAddr          string        `env:"HTTP_ADDR" envDefault:":8080"`
	ShutdownTimeout   time.Duration `env:"SHUTDOWN_TIMEOUT" envDefault:"15s"`
	ReadHeaderTimeout time.Duration `env:"READ_HEADER_TIMEOUT" envDefault:"10s"`
	DatabaseURL       string        `env:"DATABASE_URL,required,notEmpty"`
	RedisURL          string        `env:"REDIS_URL,required,notEmpty"`
	OTLPEndpoint      string        `env:"OTLP_ENDPOINT"`
	LogLevel          string        `env:"LOG_LEVEL" envDefault:"info"`
	Retry             RetryConfig   `envPrefix:"RETRY_"`
	Stream            StreamConfig  `envPrefix:"STREAM_"`
	Relay             RelayConfig   `envPrefix:"RELAY_"`
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
	if c.LogLevel != logLevelDebug && c.LogLevel != logLevelInfo && c.LogLevel != logLevelWarn && c.LogLevel != logLevelError {
		errs = append(errs, fmt.Errorf("CORE_LOG_LEVEL %q must be debug|info|warn|error", c.LogLevel))
	}
	if c.Retry.MaxDelay < c.Retry.BaseDelay {
		errs = append(errs, errors.New("CORE_RETRY_MAX_DELAY must be >= CORE_RETRY_BASE_DELAY"))
	}
	if c.Retry.MaxAttempts < 1 || c.Stream.MaxDeliveries < 1 {
		errs = append(errs, errors.New("CORE_RETRY_MAX_ATTEMPTS and CORE_STREAM_MAX_DELIVERIES must be >= 1"))
	}
	return errors.Join(errs...)
}
