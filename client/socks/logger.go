package socks

import (
	"context"
	"log/slog"

	"github.com/ezh0v/socks5"
)

type Logger struct {
	Logging *slog.Logger
}

func NewLogger(l *slog.Logger) *Logger {
	return &Logger{Logging: l}
}

var _ socks5.Logger = (*Logger)(nil)

func (l *Logger) Error(ctx context.Context, msg string, args ...any) {
	l.Logging.Error(msg, args...)
}

func (l *Logger) Info(ctx context.Context, msg string, args ...any) {
	l.Logging.Info(msg, args...)
}

func (l *Logger) Warn(ctx context.Context, msg string, args ...any) {
	l.Logging.Warn(msg, args...)
}
