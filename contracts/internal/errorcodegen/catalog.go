package errorcodegen

import (
	"errors"
	"fmt"
	"regexp"
	"sort"

	"gopkg.in/yaml.v3"
)

const (
	supportedVersion = 1
	minHTTPStatus    = 400
	maxHTTPStatus    = 599
)

var codePattern = regexp.MustCompile(`^[A-Z][A-Z0-9]*(_[A-Z0-9]+)*$`)

type Message struct {
	VI string `yaml:"vi"`
	EN string `yaml:"en"`
}

type ErrorDefinition struct {
	Code      string  `yaml:"code"`
	Status    int     `yaml:"status"`
	Retryable bool    `yaml:"retryable"`
	Message   Message `yaml:"message"`
}

type Catalog struct {
	Version int               `yaml:"version"`
	Errors  []ErrorDefinition `yaml:"errors"`
}

func Parse(raw []byte) (Catalog, error) {
	var catalog Catalog
	if err := yaml.Unmarshal(raw, &catalog); err != nil {
		return Catalog{}, fmt.Errorf("parse catalog: %w", err)
	}
	if catalog.Version != supportedVersion {
		return Catalog{}, fmt.Errorf("unsupported catalog version %d", catalog.Version)
	}
	seen := make(map[string]bool, len(catalog.Errors))
	for _, definition := range catalog.Errors {
		if err := validate(definition); err != nil {
			return Catalog{}, err
		}
		if seen[definition.Code] {
			return Catalog{}, fmt.Errorf("duplicate code %s", definition.Code)
		}
		seen[definition.Code] = true
	}
	sort.Slice(catalog.Errors, func(i, j int) bool { return catalog.Errors[i].Code < catalog.Errors[j].Code })
	return catalog, nil
}

func validate(definition ErrorDefinition) error {
	switch {
	case !codePattern.MatchString(definition.Code):
		return fmt.Errorf("code %q must be UPPER_SNAKE", definition.Code)
	case definition.Status < minHTTPStatus || definition.Status > maxHTTPStatus:
		return fmt.Errorf("code %s: status %d outside %d-%d", definition.Code, definition.Status, minHTTPStatus, maxHTTPStatus)
	case definition.Message.VI == "" || definition.Message.EN == "":
		return errors.New("code " + definition.Code + ": message.vi and message.en are required")
	}
	return nil
}
