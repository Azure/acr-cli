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

type manifestParameters struct {
	*rootParameters
	repoName string
}

func newManifestCmd(out io.Writer, rootParams *rootParameters) *cobra.Command {
	manifestParams := manifestParameters{rootParameters: rootParams}
	cmd := &cobra.Command{
		Use:   "manifest",
		Short: "Manage manifests",
		Long:  `Manage manifests`,
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
		Short: "List manifests",
		Long:  `List manifests`,
		RunE: func(cmd *cobra.Command, args []string) error {
			loginURL := api.LoginURL(manifestParams.registryName)
			acrClient, err := api.GetAcrCLIClientWithAuth(loginURL, manifestParams.username, manifestParams.password, manifestParams.configs)
			if err != nil {
				return err
			}
			ctx := context.Background()
			lastManifestDigest := ""
			resultManifests, err := acrClient.GetAcrManifests(ctx, manifestParams.repoName, "", lastManifestDigest)
			if err != nil {
				return errors.Wrap(err, "failed to list manifests")
			}

			fmt.Printf("Listing manifests for the %q repository:\n", manifestParams.repoName)

			for resultManifests != nil && resultManifests.ManifestsAttributes != nil {
				manifests := *resultManifests.ManifestsAttributes
				for _, manifest := range manifests {
					manifestDigest := *manifest.Digest
					fmt.Printf("%s/%s@%s\n", loginURL, manifestParams.repoName, manifestDigest)
				}

				lastManifestDigest = *manifests[len(manifests)-1].Digest
				resultManifests, err = acrClient.GetAcrManifests(ctx, manifestParams.repoName, "", lastManifestDigest)
				if err != nil {
					return errors.Wrap(err, "failed to list manifests")
				}
			}

			return nil
		},
	}

	return cmd
}

func newManifestDeleteCmd(out io.Writer, manifestParams *manifestParameters) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete tags",
		Long:  `Delete tags`,
		RunE: func(cmd *cobra.Command, args []string) error {
			loginURL := api.LoginURL(manifestParams.registryName)
			acrClient, err := api.GetAcrCLIClientWithAuth(loginURL, manifestParams.username, manifestParams.password, manifestParams.configs)
			if err != nil {
				return err
			}
			ctx := context.Background()

			for i := 0; i < len(args); i++ {
				err := acrClient.DeleteManifest(ctx, manifestParams.repoName, args[i])
				if err != nil {
					return errors.Wrap(err, "failed to delete manifests")
				}
				fmt.Printf("%s/%s@%s\n", loginURL, manifestParams.repoName, args[i])
			}

			return nil
		},
	}

	return cmd
}
