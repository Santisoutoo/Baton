// Package logx is baton's tiny logging facade over log/slog. Its whole reason
// to exist is a single rule: secrets never reach the logs. Auth headers and API
// keys are redacted at the source, so no call site can accidentally leak one.
package logx

import (
	"log/slog"
	"os"
	"strings"
)

// New returns a slog.Logger writing to stderr at the given level
// ("debug"|"info"|"warn"|"error"). Unknown levels fall back to info.
func New(level string) *slog.Logger {
	var lvl slog.Level
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn", "warning":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl}))
}

// Redact reduces a secret to a safe fingerprint for logs: never the value, only
// a hint of its shape ("sk-a…"+length). Empty stays empty.
func Redact(secret string) string {
	if secret == "" {
		return ""
	}
	secret = strings.TrimSpace(secret)
	// Drop a leading "Bearer " so we fingerprint the token, not the scheme.
	if lower := strings.ToLower(secret); strings.HasPrefix(lower, "bearer ") {
		secret = strings.TrimSpace(secret[len("bearer "):])
	}
	prefix := secret
	if len(prefix) > 4 {
		prefix = prefix[:4]
	}
	return prefix + "…(" + itoa(len(secret)) + ")"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
