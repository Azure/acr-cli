// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package main

import (
	"context"
	"fmt"
	"io"

	dockerAuth "github.com/Azure/acr-cli/auth/docker"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func newLoginCmd(out io.Writer) *cobra.Command {
	var opts loginOpts
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Login to a container registry",
		Long: `Login to a container registry

Examples:
  - Log in to an Azure Container Registry named "example"
    acr login -u username -p password example.azurecr.io
`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.hostname = args[0]
			return runLogin(opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.debug, "debug", "d", false, "debug mode")
	cmd.Flags().StringArrayVarP(&opts.configs, "config", "c", nil, "auth config paths")
	cmd.Flags().StringVarP(&opts.username, "username", "u", "", "the registry username")
	cmd.Flags().StringVarP(&opts.password, "password", "p", "", "the registry password or identity token")
	return cmd
}

func runLogin(opts loginOpts) error {
	if opts.debug {
		logrus.SetLevel(logrus.DebugLevel)
	}

	client, err := dockerAuth.NewClient(opts.configs...)
	if err != nil {
		return err
	}

	ctx := context.Background()

	if err := client.Login(ctx, opts.hostname, opts.username, opts.password); err != nil {
		return err
	}

	fmt.Println("Login Succeeded")
	return nil
}

type loginOpts struct {
	debug    bool
	configs  []string
	username string
	password string
	hostname string
}
