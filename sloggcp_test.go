package sloggcp

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"reflect"
	"testing"
)

type stringer struct{}

func (stringer) String() string {
	return "stringer"
}

type marshaller struct{}

func (marshaller) MarshalJSON() ([]byte, error) {
	return []byte(`{"key":"value"}`), nil
}

type expectSchema struct {
	Type           string          `json:"@type"`
	Message        string          `json:"message"`
	Severity       string          `json:"severity"`
	Source         testSource      `json:"logging.googleapis.com/sourceLocation"`
	Error          any             `json:"error"`
	Group          groupType       `json:"group"`
	Stringer       string          `json:"stringer"`
	Marshaller     json.RawMessage `json:"marshaller"`
	ReportLocation ReportLocation  `json:"reportLocation"`
}

type groupType struct {
	Bar   string `json:"bar"`
	Baz   int    `json:"baz"`
	Error string `json:"error"`
}

func (f groupType) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("bar", f.Bar),
		slog.Int("baz", f.Baz),
	)
}

var groupTypeTest = groupType{
	Bar: "baz",
	Baz: 42,
}

type testSource struct {
	Function string `json:"function"`
	// File and line are omitted.
}

func TestHandler(t *testing.T) {
	var buf bytes.Buffer
	dec := json.NewDecoder(&buf)
	tests := []struct {
		name string
		opts *slog.HandlerOptions
		log  func(logger *slog.Logger)
		want *expectSchema
	}{
		{
			name: "debug disabled",
			opts: nil,
			log: func(logger *slog.Logger) {
				logger.Debug("this is debug", "group", groupTypeTest)
			},
			want: nil,
		},
		{
			name: "debug enabled",
			opts: &slog.HandlerOptions{
				Level: slog.LevelDebug,
			},
			log: func(logger *slog.Logger) {
				logger.Debug("this is debug", "group", groupTypeTest)
			},
			want: &expectSchema{
				Message:  "this is debug",
				Severity: DebugSeverity,
				Group:    groupTypeTest,
			},
		},
		{
			name: "log info message, with stringer and marshaller",
			opts: nil,
			log: func(logger *slog.Logger) {
				logger.Info("this is info", "group", groupTypeTest, "stringer", stringer{}, "marshaller", marshaller{})
			},
			want: &expectSchema{
				Message:    "this is info",
				Severity:   InfoSeverity,
				Group:      groupTypeTest,
				Stringer:   "stringer",
				Marshaller: json.RawMessage(`{"key":"value"}`),
			},
		},
		{
			name: "log info message, with source",
			opts: &slog.HandlerOptions{
				AddSource: true,
			},
			log: func(logger *slog.Logger) {
				logger.Info("this is info", "group", groupTypeTest)
			},
			want: &expectSchema{
				Message:  "this is info",
				Severity: InfoSeverity,
				Source: testSource{
					Function: "github.com/zitadel/sloggcp.TestHandler.func4",
				},
				Group: groupTypeTest,
			},
		},
		{
			name: "log warn with group and attrs",
			log: func(logger *slog.Logger) {
				logger = logger.WithGroup("group")
				logger = logger.With(
					slog.String("bar", "baz"),
					slog.Int("baz", 42),
				)
				logger.Warn("warn message", slog.String("error", "grouped error"))
			},
			want: &expectSchema{
				Message:  "warn message",
				Severity: WarningSeverity,
				Group: groupType{
					Bar:   "baz",
					Baz:   42,
					Error: "grouped error",
				},
			},
		},
		{
			name: "log info grouped without attrs",
			log: func(logger *slog.Logger) {
				logger = logger.WithGroup("group")
				logger.Info("info message")
			},
			want: &expectSchema{
				Message:  "info message",
				Severity: InfoSeverity,
			},
		},
		{
			name: "log error string",
			log: func(logger *slog.Logger) {
				logger.Error("error message", "group", groupTypeTest, slog.String("error", "something went wrong"))
			},
			want: &expectSchema{
				Type:     ErrorReportTypeValue,
				Message:  "something went wrong",
				Severity: ErrorSeverity,
				Error:    "something went wrong",
				Group:    groupTypeTest,
			},
		},
		{
			name: "log error string from WithAttrs",
			log: func(logger *slog.Logger) {
				logger = logger.With(
					slog.String("error", "something went wrong"),
					slog.Any("group", groupTypeTest),
				)
				logger.Error("error message")
			},
			want: &expectSchema{
				Type:     ErrorReportTypeValue,
				Message:  "something went wrong",
				Severity: ErrorSeverity,
				Error:    "something went wrong",
				Group:    groupTypeTest,
			},
		},
		{
			name: "log error string after ReplaceAttr",
			opts: &slog.HandlerOptions{
				ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
					if len(groups) != 0 {
						return a
					}
					if a.Key == "err" {
						a.Key = ErrorKey
					}
					return a
				},
			},
			log: func(logger *slog.Logger) {
				logger = logger.With(
					slog.String("error", "something went wrong"),
				)
				logger.Error("error message", "group", groupTypeTest)
			},
			want: &expectSchema{
				Type:     ErrorReportTypeValue,
				Message:  "something went wrong",
				Severity: ErrorSeverity,
				Error:    "something went wrong",
				Group:    groupTypeTest,
			},
		},
		{
			name: "log standard error",
			log: func(logger *slog.Logger) {
				logger.Error("error message", "error", errors.New("something went wrong"))
			},
			want: &expectSchema{
				Type:     ErrorReportTypeValue,
				Message:  "something went wrong",
				Severity: ErrorSeverity,
				Error:    "something went wrong",
			},
		},
		{
			name: "log ReportLocationError",
			log: func(logger *slog.Logger) {
				logger.Error("error message", "error", mockReportLocationError{})
			},
			want: &expectSchema{
				Type:           ErrorReportTypeValue,
				Message:        "mockReportLocationError",
				Severity:       ErrorSeverity,
				Error:          "mockReportLocationError",
				ReportLocation: mockReportLocation,
			},
		},
		{
			name: "log StackTraceError",
			log: func(logger *slog.Logger) {
				logger.Error("error message", "error", mockStackTraceError{})
			},
			want: &expectSchema{
				Type:     ErrorReportTypeValue,
				Message:  "stack",
				Severity: ErrorSeverity,
				Error:    "mockStackTraceError",
			},
		},
		{
			name: "log stackAndReport",
			log: func(logger *slog.Logger) {
				logger.Error("error message", "error", mockStackAndReport{})
			},
			want: &expectSchema{
				Type:           ErrorReportTypeValue,
				Message:        "stack",
				Severity:       ErrorSeverity,
				Error:          "mockStackAndReport",
				ReportLocation: mockReportLocation,
			},
		},
		{
			name: "log stackAndReportValuer",
			log: func(logger *slog.Logger) {
				logger.Error("error message", "error", mockStackAndReportValuer{})
			},
			want: &expectSchema{
				Type:     ErrorReportTypeValue,
				Message:  "stack",
				Severity: ErrorSeverity,
				Error: map[string]any{
					"key1": "value1",
					"key2": float64(42),
				},
				ReportLocation: mockReportLocation,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer buf.Reset()

			h := NewErrorReportingHandler(&buf, tt.opts)
			logger := slog.New(h)
			tt.log(logger)
			if tt.want == nil {
				if buf.Len() != 0 {
					t.Errorf("log wrote data, but want is nil: %q", buf.String())
				}
				return
			}

			var got expectSchema
			if err := dec.Decode(&got); err != nil {
				t.Fatalf("Failed to decode log output: %v", err)
			}
			if !reflect.DeepEqual(&got, tt.want) {
				t.Errorf("log output = %+v, want %+v", &got, tt.want)
			}
		})
	}
}

func Test_severityFromLevel(t *testing.T) {
	tests := []struct {
		name  string
		level slog.Level
		want  string
	}{
		{
			name:  "Debug",
			level: LevelDebug,
			want:  DebugSeverity,
		},
		{
			name:  "Info",
			level: LevelInfo,
			want:  InfoSeverity,
		},
		{
			name:  "Notice",
			level: LevelNotice,
			want:  NoticeSeverity,
		},
		{
			name:  "Warning",
			level: LevelWarning,
			want:  WarningSeverity,
		},
		{
			name:  "Error",
			level: LevelError,
			want:  ErrorSeverity,
		},
		{
			name:  "Critical",
			level: LevelCritical,
			want:  CriticalSeverity,
		},
		{
			name:  "Alert",
			level: LevelAlert,
			want:  AlertSeverity,
		},
		{
			name:  "Emergency",
			level: LevelEmergency,
			want:  EmergencySeverity,
		},
		{
			name:  "Default",
			level: Level(-10),
			want:  DefaultSeverity,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := severityFromLevel(tt.level)
			if got != tt.want {
				t.Errorf("severityFromLevel() = %v, want %v", got, tt.want)
			}
		})
	}
}
