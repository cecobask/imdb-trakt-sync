package logger

import (
	"log/slog"
	"os"
)

const keyError = "error"

func NewLogger() *slog.Logger {
	opts := &slog.HandlerOptions{
		AddSource: true,
		Level:     slog.LevelInfo,
	}
	handler := slog.NewJSONHandler(os.Stdout, opts)
	return slog.New(handler)
}

func Error(err error) slog.Attr {
	return slog.Any(keyError, err)
}
