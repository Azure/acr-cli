// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package main

import (
	"context"
	"fmt"

	"github.com/Azure/acr-cli/acr"
	"github.com/Azure/acr-cli/cmd/api"
	"github.com/pkg/errors"
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
			tagList, err := listTags(ctx, acrClient, tagParams.repoName)
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

// listTags will do the http requests and return the digest of all the tags in the selected repository.
func listTags(ctx context.Context, acrClient api.AcrCLIClientInterface, repoName string) ([]acr.TagAttributesBase, error) {

	lastTag := ""
	resultTags, err := acrClient.GetAcrTags(ctx, repoName, "", lastTag)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list tags")
	}

	var tagList []acr.TagAttributesBase
	tagList = append(tagList, *resultTags.TagsAttributes...)

	// A for loop is used because the GetAcrTags method returns by default only 100 tags and their attributes.
	for resultTags != nil && resultTags.TagsAttributes != nil {
		tags := *resultTags.TagsAttributes

		// Since the GetAcrTags supports pagination when supplied with the last digest that was returned the last tag name
		// digest is saved, the tag array contains at least one element because if it was empty the API would return
		// a nil pointer instead of a pointer to a length 0 array.
		lastTag = *tags[len(tags)-1].Name
		resultTags, err = acrClient.GetAcrTags(ctx, repoName, "", lastTag)
		if err != nil {
			return nil, err
		}
		if resultTags != nil && resultTags.TagsAttributes != nil {
			tagList = append(tagList, *resultTags.TagsAttributes...)
		}
	}

	return tagList, nil
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
			err = deleteTags(ctx, acrClient, loginURL, tagParams.repoName, args)
			if err != nil {
				return err
			}
			return nil
		},
	}

	return cmd
}

// deleteTags receives an array of tags digest and deletes them using the supplied acrClient.
func deleteTags(ctx context.Context, acrClient api.AcrCLIClientInterface, loginURL string, repoName string, args []string) error {
	for i := 0; i < len(args); i++ {
		_, err := acrClient.DeleteAcrTag(ctx, repoName, args[i])
		if err != nil {
			// If there is an error (this includes not found and not allowed operations) the deletion of the tags is stopped and an error is returned.
			return errors.Wrap(err, "failed to delete tags")
		}
		fmt.Printf("%s/%s:%s\n", loginURL, repoName, args[i])
	}
	return nil
}
