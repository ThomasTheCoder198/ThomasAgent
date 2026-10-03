package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

const (
	environmentFileVariable = "CORE_ENV_FILE"
	defaultEnvironmentFile  = ".env"
)

func environmentValues() (map[string]string, error) {
	path, explicitlyConfigured := os.LookupEnv(environmentFileVariable)
	if !explicitlyConfigured {
		path = defaultEnvironmentFile
	}
	values := make(map[string]string)
	if path != "" {
		fileValues, err := readEnvironmentFile(path, explicitlyConfigured)
		if err != nil {
			return nil, err
		}
		values = fileValues
	}
	// Container configuration and explicit shell overrides must win without changing process-global state.
	for _, entry := range os.Environ() {
		key, value, _ := strings.Cut(entry, "=")
		values[key] = value
	}
	return values, nil
}

func readEnvironmentFile(path string, required bool) (map[string]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if !required && errors.Is(err, fs.ErrNotExist) {
			return make(map[string]string), nil
		}
		return nil, fmt.Errorf("read CORE_ENV_FILE %q: %w", path, err)
	}
	values, err := godotenv.Unmarshal(string(raw))
	if err != nil {
		// Dotenv parse errors can quote file contents, including credentials.
		return nil, fmt.Errorf("parse CORE_ENV_FILE %q: invalid dotenv syntax", path)
	}
	return values, nil
}
