// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package logger

import (
	"os"
	"strings"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Config holds the logger configuration
type Config struct {
	Level  string
	Format string
}

// Setup configures the global logger based on the provided config
func Setup(config Config) {
	// Set log level
	level := parseLogLevel(config.Level)
	zerolog.SetGlobalLevel(level)

	// Set log format
	if strings.ToLower(config.Format) == "console" {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	} else {
		// Default to JSON format
		log.Logger = zerolog.New(os.Stderr).With().Timestamp().Logger()
	}
}

// parseLogLevel converts string level to zerolog level
func parseLogLevel(level string) zerolog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return zerolog.DebugLevel
	case "info":
		return zerolog.InfoLevel
	case "warn", "warning":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	default:
		return zerolog.InfoLevel // Default to info
	}
}

// Common log field constants to avoid duplication and typos
const (
	FieldRepository     = "repository"
	FieldManifest       = "manifest"
	FieldTag            = "tag"
	FieldDryRun         = "dry_run"
	FieldReason         = "reason"
	FieldStatusCode     = "status_code"
	FieldRef            = "ref"
	FieldArtifactType   = "artifact_type"
	FieldManifestCount  = "manifest_count"
	FieldTagCount       = "tag_count"
	FieldDeletedCount   = "deleted_count"
	FieldAttemptedCount = "attempted_count"
	FieldMediaType      = "media_type"
)

// Get returns the global logger
func Get() *zerolog.Logger {
	return &log.Logger
}