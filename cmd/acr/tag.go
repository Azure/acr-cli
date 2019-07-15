// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package main

import (
	"context"
	"fmt"
	"io"

	dockerAuth "github.com/Azure/acr-cli/auth/docker"

	"github.com/Azure/acr-cli/cmd/api"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

type tagParameters struct {
	registryName string
	username     string
	password     string
	configs      []string
}

type tagListParameters struct {
	repository string
}

var (
	tagParams     tagParameters
	tagListParams tagListParameters
)

func newTagCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tag",
		Short: "Manage tags",
		Long:  `Manage tags`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.Help()
			return nil
		},
	}

	listTagCmd := newTagListCmd(out)
	listTagCmd.Flags().StringVar(&tagListParams.repository, "repository", "", "The name of the repository")

	cmd.AddCommand(
		listTagCmd,
		newTagDeleteCmd(out),
	)

	cmd.Flags().StringArrayVarP(&tagParams.configs, "config", "c", nil, "auth config paths")
	cmd.PersistentFlags().StringVarP(&tagParams.registryName, "registry", "r", "", "Registry name")
	cmd.PersistentFlags().StringVarP(&tagParams.username, "username", "u", "", "Registry username")
	cmd.PersistentFlags().StringVarP(&tagParams.password, "password", "p", "", "Registry password")
	cmd.MarkPersistentFlagRequired("registry")

	return cmd
}

func newTagListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List tags",
		Long:  `List tags`,
		RunE: func(cmd *cobra.Command, args []string) error {

			loginURL := api.LoginURL(tagParams.registryName)

			ctx := context.Background()
			var acrClient api.AcrCLIClient

			if tagParams.username == "" && tagParams.password == "" {
				client, err := dockerAuth.NewClient(tagParams.configs...)
				if err != nil {
					return err
				}
				tagParams.username, tagParams.password, err = client.GetCredential(loginURL)
				if err != nil {
					return err
				}
			}
			if tagParams.username == "" {
				var err error
				acrClient, err = api.NewAcrCLIClientWithBearerAuth(loginURL, tagParams.password)
				if err != nil {
					return errors.Wrap(err, "failed to list tags")
				}
			} else {
				acrClient = api.NewAcrCLIClientWithBasicAuth(loginURL, tagParams.username, tagParams.password)
			}
			lastTag := ""
			resultTags, err := acrClient.GetAcrTags(ctx, tagListParams.repository, "", lastTag)
			if err != nil {
				return errors.Wrap(err, "failed to list tags")
			}

			fmt.Printf("Listing tags for the %q repository:\n", tagListParams.repository)

			for resultTags != nil && resultTags.TagsAttributes != nil {
				tags := *resultTags.TagsAttributes
				for _, tag := range tags {
					tagName := *tag.Name
					fmt.Println(tagName)
				}

				lastTag = *tags[len(tags)-1].Name
				resultTags, err = acrClient.GetAcrTags(ctx, tagListParams.repository, "", lastTag)
				if err != nil {
					return err
				}
			}

			return nil
		},
	}

	return cmd
}

func newTagDeleteCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete tags",
		Long:  `Delete tags`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}

	return cmd
}
