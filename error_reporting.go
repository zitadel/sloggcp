package sloggcp

import (
	"errors"
	"fmt"
	"log/slog"
	"runtime"
	"strings"

	_ "runtime/debug"
)

// Key by which errors are retrieved from slog attributes.
// The corresponding values can be of type [string], [error], [StackTraceError] and/or [ReportLocationError].
const (
	ErrorKey = "error"
)

// Constants for GCP error reporting attributes.
// See https://cloud.google.com/error-reporting/docs/formatting-error-messages.
const (
	ErrorReportTypeKey   = "@type"
	ErrorReportTypeValue = "type.googleapis.com/google.devtools.clouderrorreporting.v1beta1.ReportedErrorEvent"
	ReportLocationKey    = "reportLocation"
	FilePathKey          = "filePath"
	LineNumberKey        = "lineNumber"
	FunctionNameKey      = "functionName"
)

// StackTraceError is an error that provides a stack trace,
// from the point where the error was created.
type StackTraceError interface {
	error
	// StackTrace returns the stack trace as returned by [debug.Stack].
	// If the error does not have a stack trace, ok is false.
	StackTrace() (trace []byte, ok bool)
}

// ReportLocationError is an error that provides report location information.
type ReportLocationError interface {
	error
	// ReportLocation returns the report location information, from where the error was created.
	// If the error does not have report location information, nil may be returned.
	ReportLocation() *ReportLocation
}

// assertErrorValue inspects the given value and tries to extract
// the error message and report location information.
// Supported value types are:
//   - string
//   - error (including [StackTraceError] and [ReportLocationError])
//
// For unsupported types, a generic error message is returned.
// If the error contains a stack trace, the error message is kept as header,
// followed by the stack trace separated by a newline.
func assertErrorValue(value any) (_ string, reportLocation *ReportLocation) {
	// String type won't match any other type assertions below,
	// so we can return early.
	if v, ok := value.(string); ok {
		return v, nil
	}

	err, ok := value.(error)
	if !ok {
		return fmt.Sprintf("sloggcp: unsupported type %T for error with value %v", value, value),
			NewReportLocation(0)
	}

	var msgBuf strings.Builder
	msgBuf.WriteString(err.Error())

	var ste StackTraceError
	if errors.As(err, &ste) {
		if trace, traceOk := ste.StackTrace(); traceOk {
			msgBuf.Grow(len(trace) + 1)
			msgBuf.WriteByte('\n')
			msgBuf.Write(trace)
		}
	}

	var rpe ReportLocationError
	if errors.As(err, &rpe) {
		reportLocation = rpe.ReportLocation()
	}
	return msgBuf.String(), reportLocation
}

type ReportLocation struct {
	FilePath     string `json:"filePath"`
	LineNumber   int    `json:"lineNumber"`
	FunctionName string `json:"functionName"`
}

// NewReportLocation based on the current call stack.
// The returned [ReportLocation] can be stored and returned
// in a [ReportLocationError].
// The skip parameter is the number of stack frames to skip
// (0 identifies the caller of NewReportLocation).
func NewReportLocation(skip int) *ReportLocation {
	pc, file, line, ok := runtime.Caller(skip + 1)
	fn := runtime.FuncForPC(pc)
	if !ok || fn == nil {
		return nil
	}
	return &ReportLocation{
		FilePath:     file,
		LineNumber:   line,
		FunctionName: fn.Name(),
	}
}

// LogValue implements [slog.LogValuer].
// It allows a ReportLocation to be used directly in other handlers.
func (r *ReportLocation) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String(FilePathKey, r.FilePath),
		slog.Int(LineNumberKey, r.LineNumber),
		slog.String(FunctionNameKey, r.FunctionName),
	)
}

func checkAndSetErrorReport(a slog.Attr, out map[string]any) bool {
	if a.Key != ErrorKey {
		return false
	}
	value := a.Value.Any()
	errMsg, reportLocation := assertErrorValue(value)
	out[ErrorReportTypeKey] = ErrorReportTypeValue
	out[MessageKey] = errMsg
	out[ErrorKey] = value
	if reportLocation != nil {
		out[ReportLocationKey] = reportLocation
	}
	switch v := value.(type) {
	case slog.LogValuer:
		out[ErrorKey] = extractValue(v.LogValue())
	case error:
		out[ErrorKey] = v.Error()
	}

	return true
}
