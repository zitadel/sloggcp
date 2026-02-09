package sloggcp

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"os"
	"testing"
	"time"
)

var (
	someTime   = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	someSource = slog.Source{
		Function: "test",
		File:     "test.go",
		Line:     1,
	}
)

// ExampleReplaceAttr shows how to replace default slog attributes with GCP compatible ones
// It writes to stderr, that is why output is empty
func ExampleReplaceAttr() {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		ReplaceAttr: ReplaceAttr,
		AddSource:   true,
		Level:       slog.LevelDebug,
	}))

	logger.Debug("test",
		slog.String("test", "test"),
	)

	// Output:

}

func TestReplaceAttr(t *testing.T) {
	type args struct {
		groups []string
		a      slog.Attr
	}
	tests := []struct {
		name string
		args args
		want slog.Attr
	}{
		{
			name: "TimeKey",
			args: args{
				groups: []string{},
				a:      slog.Time(slog.TimeKey, someTime),
			},
			want: slog.Time("time", someTime),
		},
		{
			name: "LevelKey Debug",
			args: args{
				groups: []string{},
				a:      slog.Any(slog.LevelKey, slog.LevelDebug),
			},
			want: slog.String("severity", "DEBUG"),
		},
		{
			name: "LevelKey Info",
			args: args{
				groups: []string{},
				a:      slog.Any(slog.LevelKey, slog.LevelInfo),
			},
			want: slog.String("severity", "INFO"),
		},
		{
			name: "LevelKey Warn",
			args: args{
				groups: []string{},
				a:      slog.Any(slog.LevelKey, slog.LevelWarn),
			},
			want: slog.String("severity", "WARNING"),
		},
		{
			name: "LevelKey Error",
			args: args{
				groups: []string{},
				a:      slog.Any(slog.LevelKey, slog.LevelError),
			},
			want: slog.String("severity", "ERROR"),
		},
		{
			name: "LevelKey Invalid level",
			args: args{
				groups: []string{},
				a:      slog.Any(slog.LevelKey, slog.Level(-1)),
			},
			want: slog.String("severity", "DEFAULT"),
		},
		{
			name: "LevelKey Invalid type",
			args: args{
				groups: []string{},
				a:      slog.Any(slog.LevelKey, "invalid"),
			},
			want: slog.String("severity", "DEFAULT"),
		},
		{
			name: "SourceKey",
			args: args{
				groups: []string{},
				a:      slog.Any(slog.SourceKey, &someSource),
			},
			want: slog.Any("logging.googleapis.com/sourceLocation", &someSource),
		},
		{
			name: "MessageKey",
			args: args{
				groups: []string{},
				a:      slog.String(slog.MessageKey, "test"),
			},
			want: slog.String("message", "test"),
		},
		{
			name: "TraceID",
			args: args{
				groups: []string{},
				a:      slog.String("TraceID", "trace-123"),
			},
			want: slog.String("trace", "trace-123"),
		},
		{
			name: "SpanID",
			args: args{
				groups: []string{},
				a:      slog.String("SpanID", "span-123"),
			},
			want: slog.String("spanId", "span-123"),
		},
		{
			name: "OtherKey",
			args: args{
				groups: []string{},
				a:      slog.String("test", "test"),
			},
			want: slog.String("test", "test"),
		},
		{
			name: "Nested LevelKey",
			args: args{
				groups: []string{"nested"},
				a:      slog.Any(slog.LevelKey, slog.LevelInfo),
			},
			want: slog.Any(slog.LevelKey, slog.LevelInfo),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ReplaceAttr(tt.args.groups, tt.args.a); !got.Equal(tt.want) {
				t.Errorf("ReplaceAttr() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestReplaceAttr_LogOutput(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		ReplaceAttr: ReplaceAttr,
		AddSource:   true,
		Level:       slog.LevelDebug,
	}))
	out := json.NewDecoder(&buf)

	tests := []struct {
		name         string
		level        slog.Level
		wantSeverity string
	}{
		{
			name:         "Debug",
			level:        slog.LevelDebug,
			wantSeverity: DebugSeverity,
		},
		{
			name:         "Info",
			level:        slog.LevelInfo,
			wantSeverity: InfoSeverity,
		},
		{
			name:         "Warn",
			level:        slog.LevelWarn,
			wantSeverity: WarningSeverity,
		},
		{
			name:         "Error",
			level:        slog.LevelError,
			wantSeverity: ErrorSeverity,
		},
		{
			name:         "Default",
			level:        slog.Level(-1),
			wantSeverity: DefaultSeverity,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer buf.Reset()
			logger.Log(t.Context(), tt.level, "test message")
			got := make(map[string]any)
			if err := out.Decode(&got); err != nil {
				t.Fatalf("Failed to decode log output: %v", err)
			}
			wantKeys := []string{"severity", "message", "time", "logging.googleapis.com/sourceLocation"}
			for _, k := range wantKeys {
				if _, ok := got[k]; !ok {
					t.Errorf("Missing key %q in log output", k)
				}
			}
			if got["severity"] != tt.wantSeverity {
				t.Errorf("severity = %v, want %v", got["severity"], tt.wantSeverity)
			}
			if got["message"] != "test message" {
				t.Errorf("message = %v, want %v", got["message"], "test message")
			}
		})

	}
}
