package command

import (
	"context"

	"github.com/Azure/acr-cli/cmd/api/option"
	"github.com/Azure/acr-cli/cmd/api/trace"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// GetLogger returns a new FieldLogger and an associated Context derived from command context.
func GetLogger(cmd *cobra.Command, opts *option.Common) (context.Context, logrus.FieldLogger) {
	ctx, logger := trace.NewLogger(cmd.Context(), opts.Debug, opts.Verbose)
	cmd.SetContext(ctx)
	return ctx, logger
}
