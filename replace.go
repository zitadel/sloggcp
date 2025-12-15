package sloggcp

import "log/slog"

// Key names handled by this package.
const (
	SourceLocationKey = "logging.googleapis.com/sourceLocation" // [slog.SourceKey] replacement
	SeverityKey       = "severity"                              // [slog.LevelKey] replacement
	MessageKey        = "message"                               // [slog.MessageKey] replacement
)

// Severity values used by GCP logging.
const (
	DebugSeverity   = "DEBUG"
	InfoSeverity    = "INFO"
	WarningSeverity = "WARNING"
	ErrorSeverity   = "ERROR"
	DefaultSeverity = "DEFAULT"
)

// ReplaceAttr replaces slog default attributes with GCP compatible ones
// https://cloud.google.com/logging/docs/structured-logging
// https://cloud.google.com/logging/docs/agent/logging/configuration#special-fields
func ReplaceAttr(groups []string, a slog.Attr) slog.Attr {

	switch {
	// TimeKey and format correspond to GCP convention by default
	// https://cloud.google.com/logging/docs/agent/logging/configuration#timestamp-processing
	case a.Key == slog.TimeKey && len(groups) == 0:
		return a
	case a.Key == slog.LevelKey && len(groups) == 0:
		logLevel, ok := a.Value.Any().(slog.Level)
		if !ok {
			return slog.String(SeverityKey, DefaultSeverity)
		}
		switch logLevel {
		case slog.LevelDebug:
			return slog.String(SeverityKey, DebugSeverity)
		case slog.LevelInfo:
			return slog.String(SeverityKey, InfoSeverity)
		case slog.LevelWarn:
			return slog.String(SeverityKey, WarningSeverity)
		case slog.LevelError:
			return slog.String(SeverityKey, ErrorSeverity)
		default:
			return slog.String(SeverityKey, DefaultSeverity)
		}
	case a.Key == slog.SourceKey && len(groups) == 0:
		source, ok := a.Value.Any().(*slog.Source)
		if !ok || source == nil {
			return a
		}
		return slog.Any(SourceLocationKey, source)
	case a.Key == slog.MessageKey && len(groups) == 0:
		return slog.String(MessageKey, a.Value.String())
	default:
		return a
	}

}
