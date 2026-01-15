package sloggcp_test

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	_ "runtime/debug"
	"time"

	"github.com/zitadel/sloggcp"
)

type AppError struct {
	Parent         error
	Message        string
	reportLocation *sloggcp.ReportLocation
	stackTrace     []byte
}

// Error implements [error].
func (e AppError) Error() string {
	return fmt.Sprintf("%s: %v", e.Message, e.Parent)
}

// ReportLocation implements [sloggcp.ReportLocationError].
func (e AppError) ReportLocation() *sloggcp.ReportLocation {
	return e.reportLocation
}

// StackTrace implements [sloggcp.StackTraceError].
func (e AppError) StackTrace() ([]byte, bool) {
	return e.stackTrace, e.stackTrace != nil
}

// LogValue implements [slog.LogValuer].
func (e AppError) LogValue() slog.Value {
	var parent slog.Attr
	if v, ok := e.Parent.(slog.LogValuer); ok {
		parent = slog.Any("parent", v)
	} else {
		parent = slog.String("parent", e.Parent.Error())
	}

	return slog.GroupValue(
		slog.String("message", e.Message),
		parent,
	)
}

func ExampleNewErrorReportingHandler() {
	h := sloggcp.NewErrorReportingHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
		// Replace "err" key with the standard [ErrorKey] for GCP error reporting.
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if len(groups) != 0 {
				return a
			}
			if a.Key == "err" {
				a.Key = sloggcp.ErrorKey
			}
			return a
		},
	})
	logger := slog.New(h).With(sloggcp.TimeKey, time.Time{}) // for deterministic output

	// Simple string error
	logger.Error("", "err", errors.New("something went wrong"))

	err := AppError{
		Parent:  errors.New("database connection failed"),
		Message: "failed to fetch user data",
		reportLocation: &sloggcp.ReportLocation{
			FilePath:     "user_service.go",
			LineNumber:   42,
			FunctionName: "fetchUserData",
		},
		stackTrace: []byte("[STACK TRACE]"), // normally call [debug.Stack]
	}
	logger.Error("", "err", err)
	// Output:
	// {"@type":"type.googleapis.com/google.devtools.clouderrorreporting.v1beta1.ReportedErrorEvent","error":"something went wrong","message":"something went wrong","severity":"ERROR","time":"0001-01-01T00:00:00Z"}
	// {"@type":"type.googleapis.com/google.devtools.clouderrorreporting.v1beta1.ReportedErrorEvent","error":{"message":"failed to fetch user data","parent":"database connection failed"},"message":"[STACK TRACE]","reportLocation":{"filePath":"user_service.go","lineNumber":42,"functionName":"fetchUserData"},"severity":"ERROR","time":"0001-01-01T00:00:00Z"}
}
