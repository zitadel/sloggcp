# sloggcp

[![Go Reference](https://pkg.go.dev/badge/github.com/zitadel/sloggcp.svg)](https://pkg.go.dev/github.com/zitadel/sloggcp)
[![codecov](https://codecov.io/github/muhlemmer/sloggcp/graph/badge.svg?token=Q1HHED6QPM)](https://codecov.io/github/muhlemmer/sloggcp)
[![Go Report Card](https://goreportcard.com/badge/github.com/zitadel/sloggcp)](https://goreportcard.com/report/github.com/zitadel/sloggcp)

`sloggcp` provides utilities to integrate Go's slog logging with Google Cloud Platform (GCP) structured logging.

## ReplaceAttr

It provides a simple implementation of the `ReplaceAttr`
function for `JSONHandler` from [slog](https://pkg.go.dev/log/slog).

This implementation adapts the default slog attributes to be compatible
with [Google Cloud Platform's Structured Logging](https://cloud.google.com/logging/docs/structured-logging), by replacing the following attribute keys:

| Slog key | GCP key                                 |
| -------- | --------------------------------------- |
| `level`  | `severity`                              |
| `msg`    | `message`                               |
| `source` | `logging.googleapis.com/sourceLocation` |
| `time`   | `time`                                  |

## Error reporting

`sloggcp` comes with a error reporting handler, which turns a log line
into a [formatted error message](https://cloud.google.com/error-reporting/docs/formatting-error-messages) whenever an error is part of the attributes.
This enables [GCP Error Reporting](https://docs.cloud.google.com/error-reporting/docs) through logging.

See the documentation for more details.

## Usage

### Get module

```sh
go get github.com/zitadel/sloggcp@latest
```

### Override default attributes

```go
package main

import (
	"github.com/zitadel/sloggcp"
	"log/slog"
	"os"
)

func main() {

	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		ReplaceAttr: sloggcp.ReplaceAttr,
		AddSource:   true,
		Level:       slog.LevelDebug,
	}))
	slog.SetDefault(logger)
}

```

### Use error reporting handler

```go
package main

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
func (e AppError) StackTrace() []byte {
	return e.stackTrace
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

func main() {
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
