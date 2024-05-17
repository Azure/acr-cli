// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package main

import (
	"context"
	"fmt"

	orasauth "github.com/Azure/acr-cli/auth/oras"
	"github.com/Azure/acr-cli/internal/cssc"

	"github.com/Azure/acr-cli/cmd/api"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

const (
	newCsscCmdLongMessage  = `acr cssc: CSSC (Container Secure Supply Chain) operations for a registry. Use subcommands for more specific operations.`
	newPatchCmdLongMessage = `acr cssc patch: List all repositories and tags that match the filter.
	Use the --filter-policy flag to specify the repository:tag where the filter exists. For v1, it should be continuouspatchpolicy:latest
	Example: acr cssc patch -r example --filter-policy continuouspatchpolicy:latest
	Use the --show-patch-tags flag to get patch tag (if it exists) for repositories and tags that match the filter.
	Example: acr cssc patch -r example --filter-policy continuouspatchpolicy:latest --show-patch-tags
	Use the --dry-run flag in combination with --file-path flag to read filter from a local file path instead of filter policy.	Use this to validate the filter file before using it as filter policy.
	Example: acr cssc patch -r example --dry-run --file-path /path/to/filter.json
	
	Filter JSON file format:
	{
		"version": "v1",
		"repositories": [
			{
				"repository": "example",
				"tags": ["tag1", "tag2"],
				"enabled": true
			}
		]
	}
`
)

type csscParameters struct {
	*rootParameters
	filterPolicy  string
	showPatchTags bool
	dryRun        bool
	filePath      string
}

// The cssc command can be used to list cssc configurations for a registry.
func newCsscCmd(rootParams *rootParameters) *cobra.Command {
	csscParams := csscParameters{rootParameters: rootParams}
	cmd := &cobra.Command{
		Use:   "cssc",
		Short: "cssc operations for a registry",
		Long:  newCsscCmdLongMessage,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.Help()
			return nil
		},
	}

	newCsscPatchFilterCmd := newPatchFilterCmd(&csscParams)

	cmd.AddCommand(
		newCsscPatchFilterCmd,
	)

	return cmd
}

// The patch subcommand can be used to list cssc continuous patch filters for a registry.
func newPatchFilterCmd(csscParams *csscParameters) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "patch",
		Short: "Manage cssc patch operations for a registry",
		Long:  newPatchCmdLongMessage,
		RunE: func(_ *cobra.Command, _ []string) error {
			ctx := context.Background()
			registryName, err := csscParams.GetRegistryName()
			if err != nil {
				return err
			}
			loginURL := api.LoginURL(registryName)
			getRegistryCredsFromStore(csscParams, loginURL)
			acrClient, err := api.GetAcrCLIClientWithAuth(loginURL, csscParams.username, csscParams.password, csscParams.configs)
			if err != nil {
				return err
			}

			filter := cssc.Filter{}
			if !csscParams.dryRun && csscParams.filePath != "" {
				return errors.New("flag --file-path can only be used in combination with --dry-run")
			} else if csscParams.dryRun {
				if csscParams.filePath == "" {
					return errors.New("flag --file-path is required when using --dry-run")
				}
				fmt.Println("DRY RUN mode enabled. Reading filter from file path...")
				filter, err = cssc.GetFilterFromFilePath(csscParams.filePath)
				if err != nil {
					return err
				}
			} else if csscParams.filterPolicy != "" {
				fmt.Println("Reading filter from filter policy...")
				filter, err = cssc.GetFilterFromFilterPolicy(ctx, csscParams.filterPolicy, loginURL, csscParams.username, csscParams.password)
				if err != nil {
					return err
				}
			}

			if len(filter.Repositories) == 0 {
				fmt.Println("Filter is empty or invalid.")
				return nil
			}
			filteredResult, err := cssc.ApplyFilterAndGetFilteredList(ctx, acrClient, filter)
			if err != nil {
				return err
			}
			cssc.PrintFilteredResult(filteredResult, csscParams.showPatchTags, loginURL)
			return nil
		},
	}

	cmd.PersistentFlags().StringVar(&csscParams.filterPolicy, "filter-policy", "", "The filter policy defined by the filter json file uploaded in a repo:tag. For v1, it should be continuouspatchpolicy:latest")
	cmd.PersistentFlags().BoolVar(&csscParams.dryRun, "dry-run", false, "Use this in combination with --file-path to read filter from file path instead of filter policy. This allows to validate the filter file without uploading it to the registry.")
	cmd.PersistentFlags().StringVar(&csscParams.filePath, "file-path", "", "The file path of the JSON filter file. Use this in combination with --dry-run to simulate the operation without making any changes on the registry.")
	cmd.Flags().BoolVar(&csscParams.showPatchTags, "show-patch-tags", false, "Use this flag to get patch tag (if it exists) for repositories and tags that match the filter. Example: acr cssc patch --filter-policy continuouspatchpolicy:latest --show-patch-tags")
	return cmd
}

func getRegistryCredsFromStore(csscParams *csscParameters, loginURL string) {
	// If both username and password are empty then the docker config file will be used, it can be found in the default
	// location or in a location specified by the configs string array
	if csscParams.username == "" || csscParams.password == "" {
		store, err := orasauth.NewStore(csscParams.configs...)
		if err != nil {
			errors.Wrap(err, "error resolving authentication")
		}
		cred, err := store.Credential(context.Background(), loginURL)
		if err != nil {
			errors.Wrap(err, "error resolving authentication")
		}
		csscParams.username = cred.Username
		csscParams.password = cred.Password

		// fallback to refresh token if it is available
		if csscParams.password == "" && cred.RefreshToken != "" {
			csscParams.password = cred.RefreshToken
		}
	}
}
