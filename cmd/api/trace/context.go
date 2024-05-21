package trace

import (
	"context"

	"github.com/sirupsen/logrus"
)

type contextKey int

// loggerKey is the associated key type for logger entry in context.
const loggerKey contextKey = iota

// NewLogger returns a logger.
func NewLogger(ctx context.Context, debug bool, verbose bool) (context.Context, logrus.FieldLogger) {
	var logLevel logrus.Level
	if debug {
		logLevel = logrus.DebugLevel
	} else if verbose {
		logLevel = logrus.InfoLevel
	} else {
		logLevel = logrus.WarnLevel
	}

	logger := logrus.New()
	logger.SetFormatter(&logrus.TextFormatter{DisableQuote: true})
	logger.SetLevel(logLevel)
	entry := logger.WithContext(ctx)
	return context.WithValue(ctx, loggerKey, entry), entry
}
