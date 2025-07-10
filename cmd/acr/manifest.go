// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package main

import (
	"context"
	"fmt"

	"github.com/Azure/acr-cli/internal/api"
	"github.com/Azure/acr-cli/internal/logger"
	"github.com/spf13/cobra"
)

const (
	newManifestCmdLongMessage       = `acr manifest: list manifests and delete them individually.`
	newManifestListCmdLongMessage   = `acr manifest list: outputs all the manifests that are inside a given repository`
	newManifestDeleteCmdLongMessage = `acr manifest delete: delete a set of manifests inside the specified repository`
)

// Besides the registry name and authentication information only the repository is needed.
type manifestParameters struct {
	*rootParameters
	repoName string
}

// The manifest command can be used to either list manifests or delete manifests inside a repository.
// that can be done with the manifest list and manifest delete commands respectively.
func newManifestCmd(rootParams *rootParameters) *cobra.Command {
	manifestParams := manifestParameters{rootParameters: rootParams}
	cmd := &cobra.Command{
		Use:   "manifest",
		Short: "Manage manifests inside a repository",
		Long:  newManifestCmdLongMessage,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.Help()
			return nil
		},
	}

	listManifestCmd := newManifestListCmd(&manifestParams)
	deleteManifestCmd := newManifestDeleteCmd(&manifestParams)

	cmd.AddCommand(
		listManifestCmd,
		deleteManifestCmd,
	)
	cmd.PersistentFlags().StringVar(&manifestParams.repoName, "repository", "", "The repository name")
	// Since the repository will be needed in either subcommand it is marked as a required flag
	cmd.MarkPersistentFlagRequired("repository")

	return cmd
}

// newManifestListCmd creates the manifest list command, it does not need any aditional parameters.
// The registry interaction is done through the listManifests method
func newManifestListCmd(manifestParams *manifestParameters) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List manifests from a repository",
		Long:  newManifestListCmdLongMessage,
		RunE: func(_ *cobra.Command, _ []string) error {
			registryName, err := manifestParams.GetRegistryName()
			if err != nil {
				return err
			}
			loginURL := api.LoginURL(registryName)
			// An acrClient is created to make the http requests to the registry.
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

// listManifests will do the http requests and print the digest of all the manifest in the selected repository.
func listManifests(ctx context.Context, acrClient api.AcrCLIClientInterface, loginURL string, repoName string) error {
	log := logger.Get().With().Str(logger.FieldRepository, repoName).Logger()
	
	log.Debug().Msg("Starting manifest listing")

	lastManifestDigest := ""
	resultManifests, err := acrClient.GetAcrManifests(ctx, repoName, "", lastManifestDigest)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get manifests for listing")
		return fmt.Errorf("failed to list manifests: %w", err)
	}

	fmt.Printf("Listing manifests for the %q repository:\n", repoName)
	
	totalManifests := 0
	// A for loop is used because the GetAcrManifests method returns by default only 100 manifests and their attributes.
	for resultManifests != nil && resultManifests.ManifestsAttributes != nil {
		manifests := *resultManifests.ManifestsAttributes
		for _, manifest := range manifests {
			manifestDigest := *manifest.Digest
			fmt.Printf("%s/%s@%s\n", loginURL, repoName, manifestDigest)
			
			log.Debug().Str(logger.FieldManifest, manifestDigest).Msg("Listed manifest")
			totalManifests++
		}
		// Since the GetAcrManifests supports pagination when supplied with the last digest that was returned the last manifest
		// digest is saved, the manifest array contains at least one element because if it was empty the API would return
		// a nil pointer instead of a pointer to a length 0 array.
		lastManifestDigest = *manifests[len(manifests)-1].Digest
		resultManifests, err = acrClient.GetAcrManifests(ctx, repoName, "", lastManifestDigest)
		if err != nil {
			log.Error().Err(err).Msg("Failed to get next page of manifests")
			return fmt.Errorf("failed to list manifests: %w", err)
		}
	}
	
	log.Info().Int("total_manifests", totalManifests).Msg("Completed manifest listing")
	return nil
}

// newManifestDeleteCmd defines the manifest delete subcommand, it receives as an argument an array of manifest digests.
// The delete functionality of this command is implemented in the deleteManifests function.
func newManifestDeleteCmd(manifestParams *manifestParameters) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete manifest from a repository",
		Long:  newManifestDeleteCmdLongMessage,
		RunE: func(_ *cobra.Command, args []string) error {
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

// deleteManifests receives an array of manifests digest and deletes them using the supplied acrClient.
func deleteManifests(ctx context.Context, acrClient api.AcrCLIClientInterface, loginURL string, repoName string, args []string) error {
	log := logger.Get().With().Str(logger.FieldRepository, repoName).Logger()
	
	log.Info().Int(logger.FieldManifestCount, len(args)).Msg("Starting manifest deletion")

	for i := 0; i < len(args); i++ {
		_, err := acrClient.DeleteManifest(ctx, repoName, args[i])
		if err != nil {
			log.Error().
				Err(err).
				Str(logger.FieldManifest, args[i]).
				Int("position", i+1).
				Int("total", len(args)).
				Msg("Failed to delete manifest")
			// If there is an error (this includes not found and not allowed operations) the deletion of the images is stopped and an error is returned.
			return fmt.Errorf("failed to delete manifests: %w", err)
		}
		
		fmt.Printf("%s/%s@%s\n", loginURL, repoName, args[i])
		
		log.Info().
			Str(logger.FieldManifest, args[i]).
			Str(logger.FieldRef, fmt.Sprintf("%s/%s@%s", loginURL, repoName, args[i])).
			Int("position", i+1).
			Int("total", len(args)).
			Msg("Successfully deleted manifest")
	}
	
	log.Info().Int(logger.FieldDeletedCount, len(args)).Msg("Completed manifest deletion")
	return nil
}
