// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package main

import (
	"context"
	"fmt"

	"github.com/Azure/acr-cli/cmd/api"
	"github.com/Azure/acr-cli/internal/tag"
	"github.com/spf13/cobra"
)

const (
	newTagCmdLongMessage       = `acr tag: list tags and untag them individually.`
	newTagListCmdLongMessage   = `acr tag list: outputs all the tags that are inside a given repository`
	newTagDeleteCmdLongMessage = `acr tag delete: delete a set of tags inside the specified repository`
)

// Besides the registry name and authentication information only the repository is needed.
type tagParameters struct {
	*rootParameters
	repoName string
}

// The tag command can be used to either list tags or delete tags inside a repository.
// that can be done with the tag list and tag delete commands respectively.
func newTagCmd(rootParams *rootParameters) *cobra.Command {
	tagParams := tagParameters{rootParameters: rootParams}
	cmd := &cobra.Command{
		Use:   "tag",
		Short: "Manage tags inside a repository",
		Long:  newTagCmdLongMessage,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.Help()
			return nil
		},
	}

	listTagCmd := newTagListCmd(&tagParams)
	deleteTagCmd := newTagDeleteCmd(&tagParams)

	cmd.AddCommand(
		listTagCmd,
		deleteTagCmd,
	)
	cmd.PersistentFlags().StringVar(&tagParams.repoName, "repository", "", "The repository name")
	// Since the repository will be needed in either subcommand it is marked as a required flag
	cmd.MarkPersistentFlagRequired("repository")

	return cmd
}

// newTagListCmd creates tag list command, it does not need any aditional parameters.
// The registry interaction is done through the listTags method
func newTagListCmd(tagParams *tagParameters) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List tags from a repository",
		Long:  newTagListCmdLongMessage,
		RunE: func(_ *cobra.Command, _ []string) error {
			registryName, err := tagParams.GetRegistryName()
			if err != nil {
				return err
			}
			loginURL := api.LoginURL(registryName)
			// An acrClient is created to make the http requests to the registry.
			acrClient, err := api.GetAcrCLIClientWithAuth(loginURL, tagParams.username, tagParams.password, tagParams.configs)
			if err != nil {
				return err
			}
			ctx := context.Background()
			tagList, err := tag.ListTags(ctx, acrClient, tagParams.repoName)
			if err != nil {
				return err
			}
			fmt.Printf("Listing tags for the %q repository:\n", tagParams.repoName)
			for _, tag := range tagList {
				fmt.Printf("%s/%s:%s\n", loginURL, tagParams.repoName, *tag.Name)
			}

			return nil
		},
	}
	return cmd
}

// newTagDeleteCmd defines the tag delete subcommand, it receives as an argument an array of tag digests.
// The delete functionality of this command is implemented in the deleteTags function.
func newTagDeleteCmd(tagParams *tagParameters) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete tags from a repository",
		Long:  newTagDeleteCmdLongMessage,
		RunE: func(_ *cobra.Command, args []string) error {
			registryName, err := tagParams.GetRegistryName()
			if err != nil {
				return err
			}
			loginURL := api.LoginURL(registryName)
			acrClient, err := api.GetAcrCLIClientWithAuth(loginURL, tagParams.username, tagParams.password, tagParams.configs)
			if err != nil {
				return err
			}
			ctx := context.Background()
			err = tag.DeleteTags(ctx, acrClient, loginURL, tagParams.repoName, args)
			if err != nil {
				return err
			}
			return nil
		},
	}

	return cmd
}
