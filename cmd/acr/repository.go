// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package main

import (
	"context"
	"fmt"
	"io"

	"github.com/Azure/acr-cli/cmd/api"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/Azure/acr-cli/cmd/worker"
)

const (
	newRepositoryCmdLongMessage       = `acr repository: list all repositories with size.`
)

// Besides the registry name and authentication information only the repository is needed.
type RepositoryParameters struct {
	*rootParameters
	concurrency int
}

// The tag command can be used to either list tags or delete tags inside a repository.
// that can be done with the tag list and tag delete commands respectively.
func newRepositoryCmd(out io.Writer, rootParams *rootParameters) *cobra.Command {
	RepositoryParams := RepositoryParameters{rootParameters: rootParams}
	cmd := &cobra.Command{
		Use:   "repo",
		Short: "Manage repositories",
		Long:  newRepositoryCmdLongMessage,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.Help()
			return nil
		},
	}

	listCmd := newListCmd(out, &RepositoryParams)

	cmd.AddCommand(
		listCmd,
	)

	cmd.Flags().IntVar(&RepositoryParams.concurrency, "concurrency", defaultPoolSize, concurrencyDescription)

	return cmd
}

// newListCmd creates list command, it does not need any aditional parameters.
// The registry interaction is done through the listRepos method
func newListCmd(out io.Writer, RepositoryParams *RepositoryParameters) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List repositories",
		Long:  newRepositoryCmdLongMessage,
		RunE: func(cmd *cobra.Command, args []string) error {
			registryName, err := RepositoryParams.GetRegistryName()
			if err != nil {
				return err
			}
			loginURL := api.LoginURL(registryName)
			// An acrClient is created to make the http requests to the registry.
			acrClient, err := api.GetAcrCLIClientWithAuth(loginURL, RepositoryParams.username, RepositoryParams.password, RepositoryParams.configs)
			if err != nil {
				return err
			}
			ctx := context.Background()
			err = listRepos(ctx, acrClient, loginURL, RepositoryParams.concurrency)
			if err != nil {
				return err
			}
			return nil
		},
	}
	return cmd
}

// listRepos uses a worker pool to crawl through all repos and calculate its size by summing up its underlying manifests
func listRepos(ctx context.Context, acrClient api.AcrCLIClientInterface, loginURL string, poolSize int) error {
	lastRepo := ""
	var maxItems int32 = 100

	Repos, err := acrClient.GetAcrRepositories(ctx, lastRepo, &maxItems)
	if err != nil {
		return errors.Wrap(err, "failed repositories")
	}

	fmt.Printf("Listing repositories:\n")
	lister := worker.NewLister(poolSize, acrClient, loginURL)
	listErr := lister.ListRepos(ctx, Repos)
	if listErr != nil {
		return errors.Wrap(err, "failed repositories")
	}
	return nil
}