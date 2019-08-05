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
	newManifestCmdLongMessage       = `acr manifest: list manifests and delete them individually.`
	newManifestListCmdLongMessage   = `acr manifest list: outputs all the manifests that are inside a given repository`
	newManifestDeleteCmdLongMessage = `acr manifest delete: delete a set of manifests inside the specified repository`
)

type manifestParameters struct {
	*rootParameters
	repoName string
}

func newManifestCmd(out io.Writer, rootParams *rootParameters) *cobra.Command {
	manifestParams := manifestParameters{rootParameters: rootParams}
	cmd := &cobra.Command{
		Use:   "manifest",
		Short: "Manage manifests inside a repository",
		Long:  newManifestCmdLongMessage,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.Help()
			return nil
		},
	}

	listManifestCmd := newManifestListCmd(out, &manifestParams)
	deleteManifestCmd := newManifestDeleteCmd(out, &manifestParams)

	cmd.AddCommand(
		listManifestCmd,
		deleteManifestCmd,
	)
	cmd.PersistentFlags().StringVar(&manifestParams.repoName, "repository", "", "The repository name")
	cmd.MarkPersistentFlagRequired("repository")

	return cmd
}

func newManifestListCmd(out io.Writer, manifestParams *manifestParameters) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List manifests from a repository",
		Long:  newManifestListCmdLongMessage,
		RunE: func(cmd *cobra.Command, args []string) error {
			registryName, err := manifestParams.GetRegistryName()
			if err != nil {
				return err
			}
			loginURL := api.LoginURL(registryName)
			acrClient, err := api.GetAcrCLIClientWithAuth(loginURL, manifestParams.username, manifestParams.password, manifestParams.configs)
			if err != nil {
				return err
			}
			ctx := context.Background()
			err = listManifests(ctx, acrClient, loginURL, manifestParams.repoName)
			if err != nil {
				return err
			}
			return nil
		},
	}

	return cmd
}

func listManifests(ctx context.Context, acrClient api.AcrCLIClientInterface, loginURL string, repoName string) error {
	lastManifestDigest := ""
	resultManifests, err := acrClient.GetAcrManifests(ctx, repoName, "", lastManifestDigest)
	if err != nil {
		return errors.Wrap(err, "failed to list manifests")
	}

	fmt.Printf("Listing manifests for the %q repository:\n", repoName)

	for resultManifests != nil && resultManifests.ManifestsAttributes != nil {
		manifests := *resultManifests.ManifestsAttributes
		for _, manifest := range manifests {
			manifestDigest := *manifest.Digest
			fmt.Printf("%s/%s@%s\n", loginURL, repoName, manifestDigest)
		}

		lastManifestDigest = *manifests[len(manifests)-1].Digest
		resultManifests, err = acrClient.GetAcrManifests(ctx, repoName, "", lastManifestDigest)
		if err != nil {
			return errors.Wrap(err, "failed to list manifests")
		}
	}
	return nil
}

func newManifestDeleteCmd(out io.Writer, manifestParams *manifestParameters) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete tags from a repository",
		Long:  newManifestDeleteCmdLongMessage,
		RunE: func(cmd *cobra.Command, args []string) error {
			registryName, err := manifestParams.GetRegistryName()
			if err != nil {
				return err
			}
			loginURL := api.LoginURL(registryName)
			acrClient, err := api.GetAcrCLIClientWithAuth(loginURL, manifestParams.username, manifestParams.password, manifestParams.configs)
			if err != nil {
				return err
			}
			ctx := context.Background()
			err = deleteManifests(ctx, acrClient, loginURL, manifestParams.repoName, args)
			if err != nil {
				return err
			}
			return nil
		},
	}

	return cmd
}

func deleteManifests(ctx context.Context, acrClient api.AcrCLIClientInterface, loginURL string, repoName string, args []string) error {
	for i := 0; i < len(args); i++ {
		_, err := acrClient.DeleteManifest(ctx, repoName, args[i])
		if err != nil {
			return errors.Wrap(err, "failed to delete manifests")
		}
		fmt.Printf("%s/%s@%s\n", loginURL, repoName, args[i])
	}
	return nil
}
