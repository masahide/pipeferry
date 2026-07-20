package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
)

func ParseLevel(value string) (slog.Level, error) {
	switch strings.ToLower(value) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("invalid log level %q", value)
	}
}

func New(out io.Writer, level slog.Level, format string) (*slog.Logger, error) {
	options := &slog.HandlerOptions{Level: level}
	switch format {
	case "text":
		return slog.New(slog.NewTextHandler(out, options)), nil
	case "json":
		return slog.New(slog.NewJSONHandler(out, options)), nil
	default:
		return nil, fmt.Errorf("invalid log format %q", format)
	}
}

func Open(path string, fallback io.Writer) (io.Writer, func() error, error) {
	if path == "" {
		return fallback, func() error { return nil }, nil
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, nil, fmt.Errorf("open log file: %w", err)
	}
	return file, file.Close, nil
}
