package logging

import (
	"log/slog"
	"reflect"
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
		return slog.AnyValue(redactContainer(v.Any()))
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

func redactContainer(value any) any {
	v := reflect.ValueOf(value)
	if !v.IsValid() {
		return value
	}
	switch v.Kind() {
	case reflect.Map:
		if v.Type().Key().Kind() != reflect.String {
			return value
		}
		out := make(map[string]any, v.Len())
		for _, key := range v.MapKeys() {
			out[key.String()] = RedactValue(key.String(), slog.AnyValue(v.MapIndex(key).Interface())).Any()
		}
		return out
	case reflect.Slice, reflect.Array:
		// Byte slices are encoded as binary strings, not structured log containers.
		if v.Type().Elem().Kind() == reflect.Uint8 || (v.Kind() == reflect.Slice && v.IsNil()) {
			return value
		}
		out := make([]any, v.Len())
		for i := range out {
			out[i] = RedactValue("", slog.AnyValue(v.Index(i).Interface())).Any()
		}
		return out
	default:
		return value
	}
}
