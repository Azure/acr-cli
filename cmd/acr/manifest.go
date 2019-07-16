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
	cmd.PersistentFlags().StringVar(&manifestParams.repoName, "repository", "", "The name of the repoName")
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

			ctx := context.Background()
			var acrClient api.AcrCLIClient

			if manifestParams.username == "" && manifestParams.password == "" {
				client, err := dockerAuth.NewClient(manifestParams.configs...)
				if err != nil {
					return err
				}
				manifestParams.username, manifestParams.password, err = client.GetCredential(loginURL)
				if err != nil {
					return err
				}
			}
			if manifestParams.username == "" {
				var err error
				acrClient, err = api.NewAcrCLIClientWithBearerAuth(loginURL, manifestParams.password)
				if err != nil {
					return errors.Wrap(err, "failed to list manifests")
				}
			} else {
				acrClient = api.NewAcrCLIClientWithBasicAuth(loginURL, manifestParams.username, manifestParams.password)
			}
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

			ctx := context.Background()
			var acrClient api.AcrCLIClient

			if manifestParams.username == "" && manifestParams.password == "" {
				client, err := dockerAuth.NewClient(manifestParams.configs...)
				if err != nil {
					return err
				}
				manifestParams.username, manifestParams.password, err = client.GetCredential(loginURL)
				if err != nil {
					return err
				}
			}
			if manifestParams.username == "" {
				var err error
				acrClient, err = api.NewAcrCLIClientWithBearerAuth(loginURL, manifestParams.password)
				if err != nil {
					return errors.Wrap(err, "failed to delete manifests")
				}
			} else {
				acrClient = api.NewAcrCLIClientWithBasicAuth(loginURL, manifestParams.username, manifestParams.password)
			}

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
