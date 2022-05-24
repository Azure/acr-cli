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
	logrus.SetFormatter(&logrus.TextFormatter{DisableQuote: true})
	logrus.SetLevel(level)
	entry := logrus.WithContext(ctx)
	return context.WithValue(ctx, loggerKey, entry), entry
}
