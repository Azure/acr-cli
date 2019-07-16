// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package main

import (
	"github.com/spf13/cobra"
)

type rootParameters struct {
	registryName string
	username     string
	password     string
	configs      []string
}

func newRootCmd(args []string) *cobra.Command {
	var rootParams rootParameters

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
		newPurgeCmd(out, &rootParams),
		newVersionCmd(out),
		newLoginCmd(out),
		newLogoutCmd(out),
		newTagCmd(out, &rootParams),
		newManifestCmd(out, &rootParams),
	)
	cmd.PersistentFlags().StringVarP(&rootParams.registryName, "registry", "r", "", "Registry name")
	cmd.PersistentFlags().StringVarP(&rootParams.username, "username", "u", "", "Registry username")
	cmd.PersistentFlags().StringVarP(&rootParams.password, "password", "p", "", "Registry password")
	cmd.Flags().StringArrayVarP(&rootParams.configs, "config", "c", nil, "Auth config paths")

	_ = flags.Parse(args)
	return cmd
}
