package config

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
)

type SchemaConfig struct {
	PostgresImage     string        `env:"POSTGRES_IMAGE" envDefault:"postgres:18.6-alpine"`
	GenerationTimeout time.Duration `env:"GENERATION_TIMEOUT" envDefault:"2m"`
	CleanupTimeout    time.Duration `env:"CLEANUP_TIMEOUT" envDefault:"15s"`
}

const schemaEnvPrefix = "CORE_SCHEMA_"

func LoadSchema() (SchemaConfig, error) {
	environment, err := environmentValues()
	if err != nil {
		return SchemaConfig{}, err
	}
	cfg, err := env.ParseAsWithOptions[SchemaConfig](env.Options{Prefix: schemaEnvPrefix, Environment: environment})
	if err != nil {
		return SchemaConfig{}, fmt.Errorf("load schema config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return SchemaConfig{}, err
	}
	return cfg, nil
}

func (c SchemaConfig) Validate() error {
	if c.PostgresImage == "" || c.GenerationTimeout <= 0 || c.CleanupTimeout <= 0 {
		return fmt.Errorf("CORE_SCHEMA_POSTGRES_IMAGE must be nonempty and schema timeouts must be positive")
	}
	return nil
}
