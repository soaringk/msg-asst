package logging

import (
	"os"
	"testing"

	"go.uber.org/zap"
)

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"debug", "debug"},
		{"DEBUG", "debug"},
		{"info", "info"},
		{"INFO", "info"},
		{"warn", "warn"},
		{"warning", "warn"},
		{"error", "error"},
		{"", "info"},
		{"invalid", "info"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			level := parseLevel(tt.input)
			if level.String() != tt.expected {
				t.Errorf("parseLevel(%q) = %v, want %v", tt.input, level.String(), tt.expected)
			}
		})
	}
}

func TestLoggerInitialization(t *testing.T) {
	os.Setenv("LOG_LEVEL", "debug")
	defer os.Unsetenv("LOG_LEVEL")

	logger := Logger()
	if logger == nil {
		t.Error("Logger() returned nil")
	}
}

func TestNamedLogger(t *testing.T) {
	logger := Named("test")
	if logger == nil {
		t.Error("Named() returned nil")
	}
}

func TestWith(t *testing.T) {
	logger := With(zap.String("key", "value"))
	if logger == nil {
		t.Error("With() returned nil")
	}
}
