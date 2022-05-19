package oras

import (
	"context"

	"github.com/sirupsen/logrus"
)

type contextKey int

// loggerKey is the associated key type for logger entry in context.
const loggerKey contextKey = iota

// WithLoggerLevel returns a context with logrus log entry.
func WithLoggerLevel(ctx context.Context, level logrus.Level) (context.Context, logrus.FieldLogger) {
	logger := logrus.New()
	logger.SetFormatter(&logrus.TextFormatter{DisableQuote: true})
	logger.SetLevel(level)
	entry := logger.WithContext(ctx)
	return context.WithValue(ctx, loggerKey, entry), entry
}

func Logger(ctx context.Context) logrus.FieldLogger {
	logger, ok := ctx.Value(loggerKey).(logrus.FieldLogger)
	if !ok {
		return logrus.StandardLogger()
	}
	return logger
}
