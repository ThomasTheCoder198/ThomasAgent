package errgen

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

type Entry struct {
	Code      string  `yaml:"code"`
	Status    int     `yaml:"status"`
	Retryable bool    `yaml:"retryable"`
	Message   Message `yaml:"message"`
}

type Catalog struct {
	Version int     `yaml:"version"`
	Errors  []Entry `yaml:"errors"`
}

func Parse(raw []byte) (Catalog, error) {
	var cat Catalog
	if err := yaml.Unmarshal(raw, &cat); err != nil {
		return Catalog{}, fmt.Errorf("parse catalog: %w", err)
	}
	if cat.Version != supportedVersion {
		return Catalog{}, fmt.Errorf("unsupported catalog version %d", cat.Version)
	}
	seen := make(map[string]bool, len(cat.Errors))
	for _, e := range cat.Errors {
		if err := validate(e); err != nil {
			return Catalog{}, err
		}
		if seen[e.Code] {
			return Catalog{}, fmt.Errorf("duplicate code %s", e.Code)
		}
		seen[e.Code] = true
	}
	sort.Slice(cat.Errors, func(i, j int) bool { return cat.Errors[i].Code < cat.Errors[j].Code })
	return cat, nil
}

func validate(e Entry) error {
	switch {
	case !codePattern.MatchString(e.Code):
		return fmt.Errorf("code %q must be UPPER_SNAKE", e.Code)
	case e.Status < minHTTPStatus || e.Status > maxHTTPStatus:
		return fmt.Errorf("code %s: status %d outside %d-%d", e.Code, e.Status, minHTTPStatus, maxHTTPStatus)
	case e.Message.VI == "" || e.Message.EN == "":
		return errors.New("code " + e.Code + ": message.vi and message.en are required")
	}
	return nil
}
