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
	newCsscCmdLongMessage  = `[Preview] acr cssc: Run cssc operations for a registry. Use subcommands for more specific operations.`
	newPatchCmdLongMessage = `[Preview] acr cssc patch: Run cssc patch operation for a registry. Use flags for more specific operations.
	
	Use the --filter-policy flag to specify the repository:tag where the filter json exists as an OCI artifact and --dry-run flag to list the filtered repositories and tags that match the filter.
	Example: acr cssc patch -r example --filter-policy continuouspatchpolicy:latest --dry-run
	
	Use the --filter-file-path flag to specify the local file path where the filter json exists and --dry-run flag to list the filtered repositories and tags that match the filter.
	Example: acr cssc patch -r example --filter-file-path /path/to/filter.json --dry-run
	
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
	filterPolicy   string
	showPatchTags  bool
	dryRun         bool
	filterfilePath string
}

// The cssc command can be used to run various cssc operations for a registry.
func newCsscCmd(rootParams *rootParameters) *cobra.Command {
	csscParams := csscParameters{rootParameters: rootParams}
	cmd := &cobra.Command{
		Use:   "cssc",
		Short: "[Preview] Run cssc operations for a registry",
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

// The patch subcommand can be used to run various patch operations for a registry.
func newPatchFilterCmd(csscParams *csscParameters) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "patch",
		Short: "[Preview] Run cssc patch operations for a registry",
		Long:  newPatchCmdLongMessage,
		RunE: func(_ *cobra.Command, _ []string) error {
			ctx := context.Background()
			registryName, err := csscParams.GetRegistryName()
			if err != nil {
				return err
			}
			loginURL := api.LoginURL(registryName)
			resolveRegistryCredentials(csscParams, loginURL)
			acrClient, err := api.GetAcrCLIClientWithAuth(loginURL, csscParams.username, csscParams.password, csscParams.configs)
			if err != nil {
				return err
			}

			filter := cssc.Filter{}
			if csscParams.filterPolicy != "" && csscParams.filterfilePath != "" {
				return errors.New("flag --filter-policy and --filter-file-path cannot be used together")
			} else if !csscParams.dryRun && csscParams.filterfilePath != "" {
				return errors.New("flag --filter-file-path can only be used in combination with --dry-run")
			} else if !csscParams.dryRun && csscParams.filterPolicy != "" {
				return errors.New("patch command without --dry-run is not operational at the moment and will be enabled in future releases")
			} else if csscParams.dryRun {
				fmt.Println("DRY RUN mode enabled...")
				if csscParams.filterPolicy == "" && csscParams.filterfilePath == "" {
					return errors.New("flag --filter-policy or --filter-file-path is required when using --dry-run")
				} else if csscParams.filterfilePath != "" {
					fmt.Println("Reading filter from filter file path...")
					filter, err = cssc.GetFilterFromFilePath(csscParams.filterfilePath)
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
	cmd.PersistentFlags().BoolVar(&csscParams.dryRun, "dry-run", false, "Use this to list the filtered repositories and tags that match the filter either from a filter policy or a filter file path. ")
	cmd.PersistentFlags().StringVar(&csscParams.filterfilePath, "filter-file-path", "", "The filter file path of the JSON filter file.")
	cmd.Flags().BoolVar(&csscParams.showPatchTags, "show-patch-tags", false, "Use this flag to get patch tag (if it exists) for repositories and tags that match the filter. Example: acr cssc patch --filter-policy continuouspatchpolicy:latest --show-patch-tags")
	return cmd
}

func resolveRegistryCredentials(csscParams *csscParameters, loginURL string) {
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
