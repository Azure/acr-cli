// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package main

import (
	"github.com/Azure/acr-cli/auth/oras"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// Similar to the login.go file but this will logout out of an ACR. This will delete
// registry credentials that are contained in a config.json file.
func newLogoutCmd() *cobra.Command {
	var opts logoutOpts
	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Log out from a container registry",
		Long: `Log out from a container registry

Examples:

  - Log out from an Azure Container Registry named "example"
    acr logout example.azurecr.io
`,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			opts.hostname = args[0]
			return runLogout(opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.debug, "debug", "d", false, "debug mode")
	cmd.Flags().StringArrayVarP(&opts.configPaths, "config", "c", nil, "auth config paths")
	return cmd
}

func runLogout(opts logoutOpts) error {
	if opts.debug {
		logrus.SetLevel(logrus.DebugLevel)
	}

	store, err := oras.NewStore()
	if err != nil {
		return err
	}

	return store.Erase(opts.hostname)
}

type logoutOpts struct {
	debug       bool
	hostname    string
	configPaths []string
}
