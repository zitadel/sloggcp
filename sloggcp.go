// Package sloggcp provides utilities to integrate Go's slog logging with Google Cloud Platform (GCP) structured logging.
package sloggcp

import (
	"context"
	"encoding"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"
)

// Keys for attributes used in GCP structured logging.
const (
	SeverityKey       = "severity"                              // [slog.LevelKey] replacement
	MessageKey        = "message"                               // [slog.MessageKey] replacement
	SourceLocationKey = "logging.googleapis.com/sourceLocation" // [slog.SourceKey] replacement
	TimeKey           = slog.TimeKey                            // time key (no replacement needed)
)

type Level = slog.Level

// Slog level aliases and extensions for GCP logging.
const (
	LevelDebug     Level = slog.LevelDebug    // Debug or trace information
	LevelInfo      Level = slog.LevelInfo     // Routine information, such as ongoing status or performance
	LevelNotice    Level = slog.LevelInfo + 2 // Normal but significant events
	LevelWarning   Level = slog.LevelWarn     // Warning events might cause problems
	LevelError     Level = slog.LevelError    // Error events are likely to cause problems
	LevelCritical  Level = LevelError + 2     // Critical events cause more severe problems or outages
	LevelAlert     Level = LevelError + 4     // A person must take an action immediately
	LevelEmergency Level = LevelError + 6     // One or more systems are unusable
)

// Severity values defined by GCP logging.
// https://docs.cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry#LogSeverity
const (
	DefaultSeverity   = "DEFAULT"   // The log entry has no assigned severity level.
	DebugSeverity     = "DEBUG"     // Debug or trace information
	InfoSeverity      = "INFO"      // Routine information, such as ongoing status or performance
	NoticeSeverity    = "NOTICE"    // Normal but significant events
	WarningSeverity   = "WARNING"   // Warning events might cause problems
	ErrorSeverity     = "ERROR"     // Error events are likely to cause problems
	CriticalSeverity  = "CRITICAL"  // Critical events cause more severe problems or outages
	AlertSeverity     = "ALERT"     // A person must take an action immediately
	EmergencySeverity = "EMERGENCY" // One or more systems are unusable
)

var DefaultOpts = slog.HandlerOptions{
	AddSource:   false,
	Level:       slog.LevelInfo,
	ReplaceAttr: nil,
}

// NewErrorReportingHandler outputs GCP compatible JSON logs to the given writer,
// Including error reporting attributes.
// Relevant Google documentation:
//   - [Structured Logging](https://cloud.google.com/logging/docs/structured-logging).
//   - [Error Reporting](https://cloud.google.com/error-reporting/docs/formatting-error-messages).
//
// Attribute values are encoded according to the following rules, in order:
//   - Attributes with [slog.KindGroup] values are expanded into nested JSON objects.
//   - Attributes with [slog.LogValuer] values are replaced by the result of their LogValue() method.
//   - Attributes with [json.Marshaler] or [encoding.TextMarshaler] values are encoded using the respective marshaling method.
//   - Attributes with [error] values are replaced by the result of their Error() method.
//   - Attributes with [fmt.Stringer] values are replaced by the result of their String() method.
//   - All other attribute values are used as-is and handled according to [json.Marshal] rules.
//
// When opts is nil, [DefaultOpts] is used.
// If ReplaceAttr is set in opts, it is called before error reporting handling.
//
// When a record contains an attribute with key [ErrorKey],
// an error report is created according to GCP error reporting specifications.
// The message attribute will then contain error details, as required by GCP error reporting.
// The passed log message is ignored.
//
// Certain attributes depend on the type of the error value.
// The "message" ([MessageKey]) attribute value is determined in the following order:
//  1. [StackTraceError] type: The stack trace output.
//  2. [string] and [error] types: The error string.
//
// The "reportLocation" ([ReportLocationKey]) attribute is added
// if the error value implements [ReportLocationError].
//
// The value associated with [ErrorKey] is determined in the following order:
//  1. [slog.LogValuer] type: The result of its LogValue() method.
//  2. [string] and [error] types: The error string.
func NewErrorReportingHandler(w io.Writer, opts *slog.HandlerOptions) slog.Handler {
	if opts == nil {
		opts = &DefaultOpts
	}
	if opts.Level == nil {
		opts.Level = DefaultOpts.Level
	}
	return &handler{
		opts:    opts,
		mtx:     new(sync.Mutex),
		encoder: json.NewEncoder(w),
	}
}

type handler struct {
	opts    *slog.HandlerOptions
	goas    []groupOrAttrs
	mtx     *sync.Mutex // protects encoder
	encoder *json.Encoder
}

// Enabled implements [slog.Handler].
func (h *handler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.opts.Level.Level()
}

// Handle implements [slog.Handler].
func (h *handler) Handle(_ context.Context, r slog.Record) error {
	n := 4 + r.NumAttrs() + len(h.goas)
	out := make(map[string]any, n)
	if !r.Time.IsZero() {
		out[TimeKey] = r.Time.Format(time.RFC3339Nano)
	}
	if h.opts.AddSource {
		if source := r.Source(); source != nil {
			out[SourceLocationKey] = source
		}
	}
	if r.Message != "" {
		out[MessageKey] = r.Message
	}
	// Handle state from WithGroup and WithAttrs.
	goas := h.goas
	out[SeverityKey] = severityFromLevel(r.Level)
	if r.NumAttrs() == 0 {
		// If the record has no Attrs, remove groups at the end of the list; they are empty.
		for len(goas) > 0 && goas[len(goas)-1].group != "" {
			goas = goas[:len(goas)-1]
		}
	}
	// Try to find error attributes only in top-level attrs.
	for _, goa := range goas {
		if goa.group != "" {
			break
		}
		for _, a := range goa.attrs {
			if checkAndSetErrorReport(a, out) {
				break
			}
		}
	}

	var (
		groups []string
		group  = out
	)
	for _, goa := range goas {
		if goa.group != "" {
			// start a new group
			newGroup := make(map[string]any)
			group[goa.group] = newGroup
			group = newGroup
			groups = append(groups, goa.group)
		} else {
			for _, a := range goa.attrs {
				a = h.replaceAttr(groups, a)
				group[a.Key] = a.Value.Any()
			}
		}
	}

	// handle record attrs
	r.Attrs(func(a slog.Attr) bool {
		a = h.replaceAttr(groups, a)
		if len(groups) == 0 {
			checkAndSetErrorReport(a, out)
		}
		group[a.Key] = extractValue(a.Value)
		return true
	})
	h.mtx.Lock()
	defer h.mtx.Unlock()
	if err := h.encoder.Encode(out); err != nil {
		return fmt.Errorf("sloggcp handler: %w", err)
	}
	return nil
}

func (h *handler) replaceAttr(groups []string, a slog.Attr) slog.Attr {
	if h.opts.ReplaceAttr != nil {
		a = h.opts.ReplaceAttr(groups, a)
	}
	return a
}

// WithAttrs implements [slog.Handler].
func (h *handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return h.withGroupOrAttrs(groupOrAttrs{attrs: attrs})
}

// WithGroup implements [slog.Handler].
func (h *handler) WithGroup(name string) slog.Handler {
	return h.withGroupOrAttrs(groupOrAttrs{group: name})
}

// groupOrAttrs holds either a group name or a list of slog.Attrs.
type groupOrAttrs struct {
	group string      // group name if non-empty
	attrs []slog.Attr // attrs if non-empty
}

func (h *handler) withGroupOrAttrs(goa groupOrAttrs) *handler {
	h2 := *h
	h2.goas = make([]groupOrAttrs, len(h.goas)+1)
	copy(h2.goas, h.goas)
	h2.goas[len(h2.goas)-1] = goa
	return &h2
}

func extractValue(v slog.Value) any {
	if v.Kind() == slog.KindGroup {
		m := make(map[string]any)
		attr := v.Group()
		for _, a := range attr {
			m[a.Key] = extractValue(a.Value)
		}
		return m
	}
	switch tv := v.Any().(type) {
	case slog.LogValuer:
		return extractValue(tv.LogValue())
	case json.Marshaler, encoding.TextMarshaler:
		return tv
	case error:
		return tv.Error()
	case fmt.Stringer:
		return tv.String()
	default:
		return tv
	}
}

func severityFromLevel(level slog.Level) string {
	if level >= LevelEmergency {
		return EmergencySeverity
	}
	if level >= LevelAlert {
		return AlertSeverity
	}
	if level >= LevelCritical {
		return CriticalSeverity
	}
	if level >= LevelError {
		return ErrorSeverity
	}
	if level >= LevelWarning {
		return WarningSeverity
	}
	if level >= LevelNotice {
		return NoticeSeverity
	}
	if level >= LevelInfo {
		return InfoSeverity
	}
	if level >= LevelDebug {
		return DebugSeverity
	}
	return DefaultSeverity
}
