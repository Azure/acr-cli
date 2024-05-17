// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package main

import (
	"errors"
	"os"

	"github.com/spf13/cobra"
)

// rootParameters defines the parameters that will be used in all of the commands.
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

	cmd.AddCommand(
		newPurgeCmd(&rootParams),
		newVersionCmd(),
		newLoginCmd(),
		newLogoutCmd(),
		newTagCmd(&rootParams),
		newManifestCmd(&rootParams),
		newCsscCmd(&rootParams),
	)
	cmd.PersistentFlags().StringVarP(&rootParams.registryName, "registry", "r", "", "Registry name")
	cmd.PersistentFlags().StringVarP(&rootParams.username, "username", "u", "", "Registry username")
	cmd.PersistentFlags().StringVarP(&rootParams.password, "password", "p", "", "Registry password")
	cmd.Flags().BoolP("help", "h", false, "Print usage")
	cmd.Flags().StringArrayVarP(&rootParams.configs, "config", "c", nil, "Auth config paths")
	// No parameter is marked as required because the registry could be inferred from a task context, same with username and password

	_ = flags.Parse(args)
	return cmd
}

// GetRegistryName if the registry flag was not specified it tries to get the registry value from an environment
// variable, if that fails then an error is returned.
func (rootParams *rootParameters) GetRegistryName() (string, error) {
	if len(rootParams.registryName) > 0 {
		return rootParams.registryName, nil
	}
	if registryName, ok := os.LookupEnv("ACR_DEFAULT_REGISTRY"); ok {
		return registryName, nil
	}
	return "", errors.New("unable to determine registry name, please use --registry flag")

}
