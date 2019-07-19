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
)

const (
	newTagCmdLongMessage       = `acr tag: list tags and untag them individually.`
	newTagListCmdLongMessage   = `acr tag list: outputs all the tags that are inside a given repository`
	newTagDeleteCmdLongMessage = `acr tag delete: delete a set of tags inside the specified repository`
)

type tagParameters struct {
	*rootParameters
	repoName string
}

func newTagCmd(out io.Writer, rootParams *rootParameters) *cobra.Command {
	tagParams := tagParameters{rootParameters: rootParams}
	cmd := &cobra.Command{
		Use:   "tag",
		Short: "Manage tags inside a repository",
		Long:  newTagCmdLongMessage,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.Help()
			return nil
		},
	}

	listTagCmd := newTagListCmd(out, &tagParams)
	deleteTagCmd := newTagDeleteCmd(out, &tagParams)

	cmd.AddCommand(
		listTagCmd,
		deleteTagCmd,
	)
	cmd.PersistentFlags().StringVar(&tagParams.repoName, "repository", "", "The repository name")
	cmd.MarkPersistentFlagRequired("repository")

	return cmd
}

func newTagListCmd(out io.Writer, tagParams *tagParameters) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List tags from a repository",
		Long:  newTagListCmdLongMessage,
		RunE: func(cmd *cobra.Command, args []string) error {
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
			lastTag := ""
			resultTags, err := acrClient.GetAcrTags(ctx, tagParams.repoName, "", lastTag)
			if err != nil {
				return errors.Wrap(err, "failed to list tags")
			}

			fmt.Printf("Listing tags for the %q repository:\n", tagParams.repoName)
			for resultTags != nil && resultTags.TagsAttributes != nil {
				tags := *resultTags.TagsAttributes
				for _, tag := range tags {
					tagName := *tag.Name
					fmt.Printf("%s/%s:%s\n", loginURL, tagParams.repoName, tagName)
				}

				lastTag = *tags[len(tags)-1].Name
				resultTags, err = acrClient.GetAcrTags(ctx, tagParams.repoName, "", lastTag)
				if err != nil {
					return err
				}
			}

			return nil
		},
	}
	return cmd
}

func newTagDeleteCmd(out io.Writer, tagParams *tagParameters) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete tags from a repository",
		Long:  newTagDeleteCmdLongMessage,
		RunE: func(cmd *cobra.Command, args []string) error {
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

			for i := 0; i < len(args); i++ {
				err := acrClient.DeleteAcrTag(ctx, tagParams.repoName, args[i])
				if err != nil {
					return errors.Wrap(err, "failed to delete tags")
				}
				fmt.Printf("%s/%s:%s\n", loginURL, tagParams.repoName, args[i])
			}

			return nil
		},
	}

	return cmd
}
