package sloggcp

import (
	"log/slog"
	"strings"
)

// ReplaceAttr replaces slog default attributes with GCP compatible ones
// https://cloud.google.com/logging/docs/structured-logging
// https://cloud.google.com/logging/docs/agent/logging/configuration#special-fields
func ReplaceAttr(groups []string, a slog.Attr) slog.Attr {
	// only handle top-level attributes
	if len(groups) > 0 {
		return a
	}
	switch {
	case a.Key == slog.LevelKey:
		return replaceLevelAttr(a)
	case a.Key == slog.SourceKey:
		a.Key = SourceLocationKey
	case a.Key == slog.MessageKey:
		a.Key = MessageKey
	case a.Key == slog.TimeKey:
		// no replacement needed
	case strings.EqualFold(a.Key, "TraceID"):
		a.Key = TraceKey
	case strings.EqualFold(a.Key, "SpanID"):
		a.Key = SpanIDKey
	}
	return a
}

var (
	severityDebug   = slog.String(SeverityKey, DebugSeverity)
	severityInfo    = slog.String(SeverityKey, InfoSeverity)
	severityWarn    = slog.String(SeverityKey, WarningSeverity)
	severityError   = slog.String(SeverityKey, ErrorSeverity)
	severityDefault = slog.String(SeverityKey, DefaultSeverity)
)

func replaceLevelAttr(a slog.Attr) slog.Attr {
	logLevel, ok := a.Value.Any().(slog.Level)
	if !ok {
		return severityDefault
	}
	switch logLevel {
	case slog.LevelDebug:
		return severityDebug
	case slog.LevelInfo:
		return severityInfo
	case slog.LevelWarn:
		return severityWarn
	case slog.LevelError:
		return severityError
	default:
		return severityDefault
	}
}
