// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package main

import (
	"github.com/spf13/cobra"
)

func newRootCmd(args []string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "acr",
		Short: "The Azure Container Registry CLI",
		Long: `Welcome to the Azure Container Registry CLI!

To start working with the CLI, run acr --help`,
		SilenceUsage: true,
	}

	flags := cmd.PersistentFlags()
	out := cmd.OutOrStdout()

	cmd.AddCommand(
		newVersionCmd(out),
	)

	_ = flags.Parse(args)
	return cmd
}
