package logger

import (
	"io"
	"log/slog"
)

const keyError = "error"

func NewLogger(writer io.Writer) *slog.Logger {
	opts := &slog.HandlerOptions{
		AddSource: true,
		Level:     slog.LevelInfo,
	}
	handler := slog.NewJSONHandler(writer, opts)
	return slog.New(handler)
}

func Error(err error) slog.Attr {
	return slog.Any(keyError, err)
}
