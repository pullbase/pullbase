package logging

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
)

type Logger = slog.Logger

type contextKey string

const loggerKey contextKey = "logger"

var defaultLogger *slog.Logger

func init() {
	defaultLogger = NewLogger(Options{})
}

type Options struct {
	Level  string
	Format string
	Output io.Writer
}

func NewLogger(opts Options) *slog.Logger {
	output := opts.Output
	if output == nil {
		output = os.Stdout
	}

	level := parseLevel(opts.Level)

	var handler slog.Handler
	handlerOpts := &slog.HandlerOptions{Level: level}

	if strings.ToLower(opts.Format) == "json" {
		handler = slog.NewJSONHandler(output, handlerOpts)
	} else {
		handler = slog.NewTextHandler(output, handlerOpts)
	}

	return slog.New(handler)
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "info", "":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func Default() *slog.Logger {
	return defaultLogger
}

func SetDefault(logger *slog.Logger) {
	defaultLogger = logger
	slog.SetDefault(logger)
}

func WithContext(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, logger)
}

func FromContext(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(loggerKey).(*slog.Logger); ok {
		return logger
	}
	return defaultLogger
}

func With(args ...any) *slog.Logger {
	return defaultLogger.With(args...)
}

func Debug(msg string, args ...any) {
	defaultLogger.Debug(msg, args...)
}

func Info(msg string, args ...any) {
	defaultLogger.Info(msg, args...)
}

func Warn(msg string, args ...any) {
	defaultLogger.Warn(msg, args...)
}

func Error(msg string, args ...any) {
	defaultLogger.Error(msg, args...)
}

func Fatal(msg string, args ...any) {
	defaultLogger.Error(msg, args...)
	os.Exit(1)
}
