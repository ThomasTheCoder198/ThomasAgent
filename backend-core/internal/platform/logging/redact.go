package logging

import (
	"log/slog"
	"regexp"
	"strings"
)

const Redacted = "[REDACTED]"

// Matching whole words keeps usage fields such as input_tokens visible while csrf_token or X-Api-Key are hidden.
var (
	secretKeyWords   = map[string]bool{"secret": true, "password": true, "passwd": true, "token": true, "authorization": true, "cookie": true, "apikey": true}
	secretKeyPhrases = []string{"api_key", "private_key"}
	keyWordSplitter  = regexp.MustCompile(`[^a-z0-9]+`)
)

var secretValuePattern = regexp.MustCompile(`(?i)(bearer\s+\S+|sk-[a-z0-9_\-]{3,}\S*)`)

func isSecretKey(key string) bool {
	k := strings.ToLower(key)
	for _, phrase := range secretKeyPhrases {
		if strings.Contains(strings.ReplaceAll(k, "-", "_"), phrase) {
			return true
		}
	}
	for _, word := range keyWordSplitter.Split(k, -1) {
		if secretKeyWords[word] {
			return true
		}
	}
	return false
}

func RedactValue(key string, v slog.Value) slog.Value {
	if isSecretKey(key) {
		return slog.StringValue(Redacted)
	}
	v = v.Resolve()
	switch v.Kind() {
	case slog.KindAny:
		if value, ok := v.Any().(map[string]any); ok {
			attrs := make([]slog.Attr, 0, len(value))
			for k, item := range value {
				attrs = append(attrs, slog.Attr{Key: k, Value: RedactValue(k, slog.AnyValue(item))})
			}
			return slog.GroupValue(attrs...)
		}
		return v
	case slog.KindString:
		return slog.StringValue(secretValuePattern.ReplaceAllString(v.String(), Redacted))
	case slog.KindGroup:
		attrs := v.Group()
		out := make([]slog.Attr, len(attrs))
		for i, a := range attrs {
			out[i] = slog.Attr{Key: a.Key, Value: RedactValue(a.Key, a.Value.Resolve())}
		}
		return slog.GroupValue(out...)
	default:
		return v
	}
}
