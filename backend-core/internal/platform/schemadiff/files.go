package schemadiff

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	migrationTimestampFormat  = "20060102150405"
	migrationNamePattern      = `^[a-z][a-z0-9]*(?:_[a-z0-9]+)*$`
	migrationExtension        = ".sql"
	migrationTemporaryPattern = ".migration-*.tmp"
)

func migrationFilename(name string, now time.Time, filenames []string) (string, error) {
	if !regexp.MustCompile(migrationNamePattern).MatchString(name) {
		return "", fmt.Errorf("migration name must use snake_case beginning with a letter")
	}
	version := now.UTC().Format(migrationTimestampFormat)
	for _, filename := range filenames {
		if !strings.HasSuffix(filename, migrationExtension) {
			continue
		}
		existingVersion, _, ok := strings.Cut(filename, "_")
		if !ok {
			return "", fmt.Errorf("migration filename %q must contain timestamp and name", filename)
		}
		if _, err := time.Parse(migrationTimestampFormat, existingVersion); err != nil {
			return "", fmt.Errorf("invalid migration timestamp in %q: %w", filename, err)
		}
		if version <= existingVersion {
			return "", fmt.Errorf("new UTC migration timestamp %s must be later than %s; wait or correct the clock", version, existingVersion)
		}
	}
	return version + "_" + name + migrationExtension, nil
}

func writeMigration(path string, contents []byte) (retErr error) {
	return publishMigration(path, func(file *os.File) error {
		_, err := file.Write(contents)
		return err
	})
}

func publishMigration(path string, write func(*os.File) error) (retErr error) {
	file, err := os.CreateTemp(filepath.Dir(path), migrationTemporaryPattern)
	if err != nil {
		return fmt.Errorf("create temporary migration: %w", err)
	}
	closed := false
	defer func() {
		if !closed {
			retErr = errors.Join(retErr, file.Close())
		}
		retErr = errors.Join(retErr, os.Remove(file.Name()))
	}()
	if err := write(file); err != nil {
		return fmt.Errorf("write migration: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("flush migration: %w", err)
	}
	err = file.Close()
	closed = true
	if err != nil {
		return fmt.Errorf("close migration: %w", err)
	}
	// Linking publishes fully written bytes and refuses an existing destination on Windows and Unix.
	if err := os.Link(file.Name(), path); err != nil {
		return fmt.Errorf("publish migration exclusively: %w", err)
	}
	return nil
}
