package logging

import (
	"bytes"
	"io"
	"log/slog"
	"strings"
	"testing"
)

func TestParseLevel(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  slog.Level
		ok    bool
	}{
		{"debug", slog.LevelDebug, true},
		{"info", slog.LevelInfo, true},
		{"warn", slog.LevelWarn, true},
		{"error", slog.LevelError, true},
		{"trace", 0, false},
	} {
		got, err := ParseLevel(tc.input)
		if (err == nil) != tc.ok || got != tc.want {
			t.Fatalf("ParseLevel(%q) = %v, %v", tc.input, got, err)
		}
	}
}

func TestJSONLogger(t *testing.T) {
	var output bytes.Buffer
	logger, err := New(&output, slog.LevelInfo, "json")
	if err != nil {
		t.Fatal(err)
	}
	logger.Info("ready")
	if !strings.Contains(output.String(), `"msg":"ready"`) {
		t.Fatalf("unexpected output: %s", output.String())
	}
}

func TestInvalidFormat(t *testing.T) {
	if _, err := New(io.Discard, slog.LevelInfo, "xml"); err == nil {
		t.Fatal("invalid format accepted")
	}
}
