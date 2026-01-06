package sloggcp

import (
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
			name:               "string value",
			value:              "oops",
			wantErrMsg:         "oops",
			wantReportLocation: nil,
		},
		{
			name:               "error value",
			value:              errors.New("oops"),
			wantErrMsg:         "oops",
			wantReportLocation: nil,
		},
		{
			name:               "ReportLocationError value",
			value:              mockReportLocationError{},
			wantErrMsg:         "mockReportLocationError",
			wantReportLocation: &mockReportLocation,
		},
		{
			name:               "StackTraceError value",
			value:              mockStackTraceError{},
			wantErrMsg:         "stack",
			wantReportLocation: nil,
		},
		{
			name:               "stackAndReport value",
			value:              mockStackAndReport{},
			wantErrMsg:         "stack",
			wantReportLocation: &mockReportLocation,
		},
		{
			name:           "unknown type value",
			value:          42,
			wantErrMsg:     "!!! can't handle error report for type int !!!",
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

type mockStackTraceError struct{}

func (m mockStackTraceError) Error() string {
	return "mockStackTraceError"
}

func (m mockStackTraceError) StackTrace() []byte {
	return []byte("stack")
}

type mockStackAndReport struct{}

func (m mockStackAndReport) Error() string {
	return "mockStackAndReport"
}

func (m mockStackAndReport) StackTrace() []byte {
	return []byte("stack")
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
