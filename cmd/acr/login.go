// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Azure/acr-cli/auth/oras"
	"github.com/moby/term"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"oras.land/oras-go/v2/registry/remote"
)

const (
	newLoginCmdLongMessage = `Login to a container registry, obtaining credentials or writing them to the config file`
	loginExampleMessage    = `  - Log in to an Azure Container Registry named "example"
    acr login -u username -p password example.azurecr.io

  - Log in to an Azure Container Registry named "example" getting the password from stdin
    acr login example.azurecr.io -u username --password-stdin

  - Log in to an Azure Container Registry named "example" from prompt
    acr login example.azurecr.io`
)

type loginOpts struct {
	hostname  string
	username  string
	password  string
	configs   []string
	debug     bool
	fromStdin bool
}

// newLoginCmd is used when the program is used locally and not inside a container.
// It is based on the login.go file found on https://github.com/deislabs/oras/blob/master/cmd/oras/login.go
// This will login into any ACR, the login info will be written in a config.json file inside the .docker folder.
func newLoginCmd() *cobra.Command {
	var opts loginOpts
	cmd := &cobra.Command{
		Use:     "login",
		Short:   "Login to a container registry",
		Long:    newLoginCmdLongMessage,
		Example: loginExampleMessage,
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			opts.hostname = args[0]
			return runLogin(opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.debug, "debug", "d", false, "debug mode")
	cmd.Flags().StringArrayVarP(&opts.configs, "config", "c", nil, "auth config paths")
	cmd.Flags().StringVarP(&opts.username, "username", "u", "", "the registry username")
	cmd.Flags().StringVarP(&opts.password, "password", "p", "", "the registry password or identity token")
	cmd.Flags().BoolVarP(&opts.fromStdin, "password-stdin", "", false, "read password or identity token from stdin")
	return cmd
}

func runLogin(opts loginOpts) error {
	if opts.debug {
		logrus.SetLevel(logrus.DebugLevel)
	}

	store, err := oras.NewStore(opts.configs...)
	if err != nil {
		return err
	}

	var username string
	var passwordBytes []byte
	if opts.fromStdin {
		passwordBytes, err = io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}
		opts.password = strings.TrimSuffix(string(passwordBytes), "\n")
		opts.password = strings.TrimSuffix(opts.password, "\r")
	} else if opts.password == "" {
		if opts.username == "" {
			username, err = readLine("Username: ", false)
			if err != nil {
				return err
			}
			opts.username = strings.TrimSpace(username)
		}
		if opts.password, err = readLine("Password: ", true); err != nil {
			return err
		} else if opts.password == "" {
			return errors.New("password required")
		}

	} else {
		fmt.Fprintln(os.Stderr, "WARNING! Using --password via the CLI is insecure. Use --password-stdin.")
	}

	// Ping to ensure credential is valid
	remote, err := remote.NewRegistry(opts.hostname)
	if err != nil {
		return err
	}
	cred := oras.Credential(opts.username, opts.password)
	remote.Client = oras.NewClient(oras.ClientOptions{
		Credential: cred,
		Debug:      opts.debug,
	})
	if err = remote.Ping(context.Background()); err != nil {
		return err
	}
	// Store the validated credential
	if err := store.Store(opts.hostname, cred); err != nil {
		return err
	}

	fmt.Println("Login Succeeded")
	return nil
}

func readLine(prompt string, silent bool) (string, error) {
	fmt.Print(prompt)
	if silent {
		fd := os.Stdin.Fd()
		state, err := term.SaveState(fd)
		if err != nil {
			return "", err
		}
		term.DisableEcho(fd, state)
		defer term.RestoreTerminal(fd, state)
	}

	reader := bufio.NewReader(os.Stdin)
	line, _, err := reader.ReadLine()
	if err != nil {
		return "", err
	}
	if silent {
		fmt.Println()
	}

	return string(line), nil
}
