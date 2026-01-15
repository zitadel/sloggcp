package sloggcp

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func Test_assertErrorValue(t *testing.T) {
	tests := []struct {
		name               string
		value              any
		wantErrMsg         string
		locationNotNil     bool
		wantReportLocation *ReportLocation
	}{
		{
			name:               "string type",
			value:              "oops",
			wantErrMsg:         "oops",
			wantReportLocation: nil,
		},
		{
			name:               "error type",
			value:              errors.New("oops"),
			wantErrMsg:         "oops",
			wantReportLocation: nil,
		},
		{
			name:               "ReportLocationError type",
			value:              mockReportLocationError{},
			wantErrMsg:         "mockReportLocationError",
			wantReportLocation: &mockReportLocation,
		},
		{
			name:               "StackTraceError type returns stack",
			value:              mockStackTraceError{true},
			wantErrMsg:         "mockStackTraceError\nstack",
			wantReportLocation: nil,
		},
		{
			name:               "StackTraceError type no stack",
			value:              mockStackTraceError{false},
			wantErrMsg:         "mockStackTraceError",
			wantReportLocation: nil,
		},
		{
			name:               "stackAndReport type returns stack and report location",
			value:              mockStackAndReport{true},
			wantErrMsg:         "mockStackAndReport\nstack",
			wantReportLocation: &mockReportLocation,
		},
		{
			name:               "stackAndReport type returns only report location",
			value:              mockStackAndReport{false},
			wantErrMsg:         "mockStackAndReport",
			wantReportLocation: &mockReportLocation,
		},
		{
			name:           "unknown type",
			value:          42,
			wantErrMsg:     "sloggcp: unsupported type int for error with value 42",
			locationNotNil: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotErrMsg, gotReportLocation := assertErrorValue(tt.value)
			if tt.wantErrMsg != gotErrMsg {
				t.Errorf("assertErrorValue() = %v, want %v", gotErrMsg, tt.wantErrMsg)
			}
			if tt.locationNotNil {
				if gotReportLocation == nil {
					t.Errorf("assertErrorValue() reportLocation = nil, want non-nil")
				}
				return
			}
			if !reflect.DeepEqual(tt.wantReportLocation, gotReportLocation) {
				t.Errorf("assertErrorValue() = %v, want %v", gotReportLocation, tt.wantReportLocation)
			}
		})
	}
}

func TestNewReportLocation(t *testing.T) {
	tests := []struct {
		name string
		skip int
		want bool
	}{
		{
			name: "OK",
			skip: 0,
			want: true,
		},
		{
			name: "nil",
			skip: 100,
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewReportLocation(tt.skip)
			_, _, wantLine, _ := runtime.Caller(0)
			wantLine-- // previous line
			if !tt.want {
				if got != nil {
					t.Errorf("NewReportLocation() = %v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("NewReportLocation() = nil, want non-nil")
			}

			const (
				wantFileSuffix = "error_reporting_test.go"
				wantFuncName   = "github.com/zitadel/sloggcp.TestNewReportLocation.func1"
			)

			if !strings.HasSuffix(got.FilePath, wantFileSuffix) {
				t.Errorf("NewReportLocation() filePath = %v, want suffix %v", got.FilePath, wantFileSuffix)
			}
			if got.LineNumber != wantLine {
				t.Errorf("NewReportLocation() lineNumber = %v, want %v", got.LineNumber, wantLine)
			}
			if got.FunctionName != wantFuncName {
				t.Errorf("NewReportLocation() functionName = %v, want %v", got.FunctionName, wantFuncName)
			}
		})
	}
}

var mockReportLocation = ReportLocation{
	FilePath:     "file.go",
	LineNumber:   42,
	FunctionName: "package.function",
}

type mockReportLocationError struct{}

func (m mockReportLocationError) Error() string {
	return "mockReportLocationError"
}

func (m mockReportLocationError) ReportLocation() *ReportLocation {
	return &mockReportLocation
}

type mockStackTraceError struct {
	returnStack bool
}

func (m mockStackTraceError) Error() string {
	return "mockStackTraceError"
}

func (m mockStackTraceError) StackTrace() ([]byte, bool) {
	if m.returnStack {
		return []byte("stack"), m.returnStack
	}
	return nil, false
}

type mockStackAndReport struct {
	returnStack bool
}

func (m mockStackAndReport) Error() string {
	return "mockStackAndReport"
}

func (m mockStackAndReport) StackTrace() ([]byte, bool) {
	if m.returnStack {
		return []byte("stack"), m.returnStack
	}
	return nil, false
}

func (m mockStackAndReport) ReportLocation() *ReportLocation {
	return &mockReportLocation
}

type mockStackAndReportValuer struct {
	mockStackAndReport
}

func (m mockStackAndReportValuer) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("key1", "value1"),
		slog.Int("key2", 42),
	)
}

func TestReportLocation_LogValue(t *testing.T) {
	type schema struct {
		Msg      string
		Level    string
		Location *ReportLocation
	}

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	location := NewReportLocation(0)
	_, _, wantLine, _ := runtime.Caller(0)
	wantLine-- // previous line
	logger.Info("test", "location", location)

	var got schema
	err := json.Unmarshal(buf.Bytes(), &got)
	if err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if got.Msg != "test" {
		t.Errorf("LogValue() Msg = %v, want %v", got.Msg, "test")
	}
	if got.Level != "INFO" {
		t.Errorf("LogValue() Level = %v, want %v", got.Level, "INFO")
	}
	if got.Location == nil {
		t.Fatal("LogValue() Location = nil, want non-nil")
	}
	if !strings.HasSuffix(got.Location.FilePath, "error_reporting_test.go") {
		t.Errorf("LogValue() Location.FilePath = %v, want suffix %v", got.Location.FilePath, "error_reporting_test.go")
	}
	if got.Location.LineNumber != wantLine {
		t.Errorf("LogValue() Location.LineNumber = %v, want %v", got.Location.LineNumber, wantLine)
	}
	if !strings.HasSuffix(got.Location.FunctionName, "TestReportLocation_LogValue") {
		t.Errorf("LogValue() Location.FunctionName = %v, want suffix %v", got.Location.FunctionName, "TestReportLocation_LogValue")
	}
}
