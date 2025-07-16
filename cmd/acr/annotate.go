// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/Azure/acr-cli/cmd/common"
	"github.com/Azure/acr-cli/internal/api"
	"github.com/Azure/acr-cli/internal/worker"
	"github.com/dlclark/regexp2"
	"github.com/spf13/cobra"
)

// The constants for the file are defined here
const (
	newAnnotateCmdLongMessage = `acr annotate: annotate individual tags for an image and untagged referrers.`
	annotateExampleMessage    = `- Annotate all images with tags that begin with hello in the example.azurecr.io registry inside the hello-world repository
	acr annotate -r example --filter "hello-world:^hello.*" --annotations "vnd.microsoft.artifact.lifecycle.end-of-life.date=2024-04-09" 
	--artifact-type "application/vnd.microsoft.artifact.lifecycle"

- Annotate all tags that contain the word test in the tag name in the example.azurecr.io registry inside the hello-world 
repository, after that, remove the dangling manifests in the same repository
acr annotate -r example --filter "hello-world:\w*test\w*" --annotations "vnd.microsoft.artifact.lifecycle.end-of-life.date=2024-04-09" 
--artifact-type "application/vnd.microsoft.artifact.lifecycle" --untagged 

- Annotate all tags that contain the word test in the tag name in the example.azurecr.io registry inside the hello-world
repository. Two annotations need to be applied.
acr annotate -r example --filter "hello-world:\w*test\w* --annotations "vnd.microsoft.artifact.lifecycle.end-of-life.date=2024-04-09" 
--annotations "key=value" --artifact-type "application/vnd.microsoft.artifact.lifecycle" 

- Annotate all tags in the example.azurecr.io registry inside the hello-world repository, with 4 annotate tasks running concurrently
acr annotate -r example --filter "hello-world:.*" --annotations "vnd.microsoft.artifact.lifecycle.end-of-life.date=2024-04-09" 
--artifact-type "application/vnd.microsoft.artifact.lifecycle" --concurrency 4
`
)

var (
	annotatedConcurrencyDescription = fmt.Sprintf("Number of concurrent annotate tasks. Range: [1 - %d]", maxPoolSize)
)

// annotateParameters defines the parameters that the annotate command uses (including the registry name, username, and password)
type annotateParameters struct {
	*rootParameters
	filters       []string
	filterTimeout int64
	artifactType  string
	annotations   []string
	untagged      bool
	dryRun        bool
	concurrency   int
	includeLocked bool
}

// newAnnotateCmd defines the annotate command
func newAnnotateCmd(rootParams *rootParameters) *cobra.Command {
	annotateParams := annotateParameters{rootParameters: rootParams}
	cmd := &cobra.Command{
		Use:     "annotate",
		Short:   "[Preview] Annotate images in a registry",
		Long:    newAnnotateCmdLongMessage,
		Example: annotateExampleMessage,
		RunE: func(_ *cobra.Command, _ []string) error {
			// This context is used for all the http requests
			ctx := context.Background()
			registryName, err := annotateParams.GetRegistryName()
			if err != nil {
				return err
			}
			loginURL := api.LoginURL(registryName)
			// An acrClient with authentication is generated, if the authentication cannot be resolved an error is returned.
			acrClient, err := api.GetAcrCLIClientWithAuth(loginURL, annotateParams.username, annotateParams.password, annotateParams.configs)
			if err != nil {
				return err
			}

			orasClient, err := api.GetORASClientWithAuth(annotateParams.username, annotateParams.password, annotateParams.configs)
			if err != nil {
				return err
			}

			// A map is used to collect the regex tags for every repository.
			tagFilters, err := common.CollectTagFilters(ctx, annotateParams.filters, acrClient.AutorestClient, annotateParams.filterTimeout, defaultRepoPageSize)
			if err != nil {
				return err
			}
			// A clarification message for --dry-run.
			if annotateParams.dryRun {
				fmt.Println("DRY RUN: The following output shows what WOULD be annotated if the annotate command was executed. Nothing is annotated.")
			}
			// In order to print a summary of the annotated tags/manifests, the counters get updated every time a repo is annotated.
			annotatedTagsCount := 0
			annotatedManifestsCount := 0
			skippedTagsCount := 0
			skippedManifestsCount := 0

			poolSize := annotateParams.concurrency
			if poolSize <= 0 {
				poolSize = defaultPoolSize
				fmt.Printf("Specified concurrency value invalid. Set to default value: %d \n", defaultPoolSize)
			} else if poolSize > maxPoolSize {
				poolSize = maxPoolSize
				fmt.Printf("Specified concurrency value too large. Set to maximum value: %d \n", maxPoolSize)
			}
			for repoName, tagRegex := range tagFilters {
				singleAnnotatedTagsCount, singleSkippedTagsCount, err := annotateTags(ctx, acrClient, orasClient, poolSize, loginURL, repoName, annotateParams.artifactType, annotateParams.annotations, tagRegex, annotateParams.filterTimeout, annotateParams.dryRun, annotateParams.includeLocked)
				if err != nil {
					return fmt.Errorf("failed to annotate tags: %w", err)
				}

				singleAnnotatedManifestsCount := 0
				var singleSkippedManifestsCount int
				// If the untagged flag is set, then manifests with no tags are also annotated..
				if annotateParams.untagged {
					singleAnnotatedManifestsCount, singleSkippedManifestsCount, err = annotateUntaggedManifests(ctx, acrClient, orasClient, poolSize, loginURL, repoName, annotateParams.artifactType, annotateParams.annotations, annotateParams.dryRun, annotateParams.includeLocked)
					if err != nil {
						return fmt.Errorf("failed to annotate manifests: %w", err)
					}
					skippedManifestsCount += singleSkippedManifestsCount
				}

				// After every repository is annotated, the counters are updated
				annotatedTagsCount += singleAnnotatedTagsCount
				annotatedManifestsCount += singleAnnotatedManifestsCount
				skippedTagsCount += singleSkippedTagsCount
			}

			// After all repos have been annotated, the summary is printed
			if annotateParams.dryRun {
				fmt.Printf("\nNumber of tags to be annotated: %d", annotatedTagsCount)
				fmt.Printf("\nNumber of manifests to be annotated: %d\n", annotatedManifestsCount)
			} else {
				fmt.Printf("\nNumber of annotated tags: %d", annotatedTagsCount)
				fmt.Printf("\nNumber of annotated manifests: %d\n", annotatedManifestsCount)
			}
			if skippedTagsCount > 0 {
				fmt.Printf("%d tags skipped as they are locked\n", skippedTagsCount)
			}
			if skippedManifestsCount > 0 {
				fmt.Printf("%d manifests skipped as they are locked\n", skippedManifestsCount)
			}
			return nil
		},
	}

	cmd.Flags().StringArrayVarP(&annotateParams.filters, "filter", "f", nil, `Specify the repository and a regular expression filter for the tag name. If a tag matches the filter, it will be annotated. If multiple tags refer to the same manifest and a tag matches the filter, the manifest will be annotated.
																				Note: If backtracking is used in the regexp it's possible for the expression to run into an infinite loop. The default timeout is set to 1 minute for evaluation of any filter expression. Use the '--filter-timeout-seconds' option to set a different value`)
	cmd.Flags().Int64Var(&annotateParams.filterTimeout, "filter-timeout-seconds", defaultRegexpMatchTimeoutSeconds, "This limits the evaluation of the regex filter, and will return a timeout error if this duration is exceeded during a single evaluation. If written incorrectly a regexp filter with backtracking can result in an infinite loop")
	cmd.Flags().StringVar(&annotateParams.artifactType, "artifact-type", "", "The configurable artifact type for an organization")
	cmd.Flags().StringSliceVarP(&annotateParams.annotations, "annotations", "a", []string{}, "The configurable annotation key value that can be specified one or more times")
	cmd.Flags().BoolVar(&annotateParams.untagged, "untagged", false, "If the untagged flag is set, all the manifests that do not have any tags associated to them will also be annotated, except if they belong to a manifest list that contains at least one tag")
	cmd.Flags().BoolVar(&annotateParams.dryRun, "dry-run", false, "If the dry-run flag is set, no manifest or tag will be annotated. The output would be the same as if they were annotated")
	cmd.Flags().BoolVar(&annotateParams.includeLocked, "include-locked", false, "If the include-locked flag is set, locked manifests and tags (where writeEnabled is false) will be annotated")
	cmd.Flags().IntVar(&annotateParams.concurrency, "concurrency", defaultPoolSize, annotatedConcurrencyDescription)
	cmd.Flags().BoolP("help", "h", false, "Print usage")
	cmd.MarkFlagRequired("filter")
	cmd.MarkFlagRequired("artifact-type")
	cmd.MarkFlagRequired("annotations")
	return cmd
}

// annotateTags annotates all tags that match the tagFilter string.
func annotateTags(ctx context.Context,
	acrClient api.AcrCLIClientInterface,
	orasClient api.ORASClientInterface,
	poolSize int,
	loginURL string,
	repoName string,
	artifactType string,
	annotations []string,
	tagFilter string,
	regexpMatchTimeoutSeconds int64,
	dryRun bool,
	includeLocked bool) (int, int, error) {

	if !dryRun {
		fmt.Printf("\nAnnotating tags for repository: %s\n", repoName)
	} else {
		fmt.Printf("\nTags for this repository would be annotated: %s\n", repoName)
	}

	tagRegex, err := common.BuildRegexFilter(tagFilter, regexpMatchTimeoutSeconds)
	if err != nil {
		return -1, 0, err
	}

	lastTag := ""
	annotatedTagsCount := 0
	totalSkippedCount := 0

	var annotator *worker.Annotator
	if !dryRun {
		// In order to only have a limited amount of http requests, an annotator is used that will start goroutines to annotate tags.
		annotator, err = worker.NewAnnotator(poolSize, orasClient, loginURL, repoName, artifactType, annotations)
		if err != nil {
			return -1, 0, err
		}
	}

	for {
		// GetTagsToAnnotate will return an empty lastTag when there are no more tags.
		manifestsToAnnotate, newLastTag, skippedCount, err := getManifestsToAnnotate(ctx, acrClient, orasClient, loginURL, repoName, tagRegex, lastTag, artifactType, dryRun, includeLocked)
		if err != nil {
			return -1, 0, err
		}
		lastTag = newLastTag
		count := len(manifestsToAnnotate)
		totalSkippedCount += skippedCount
		if count > 0 {
			if !dryRun {
				annotated, annotateErr := annotator.Annotate(ctx, manifestsToAnnotate)
				if annotateErr != nil {
					annotatedTagsCount += annotated
					return annotatedTagsCount, totalSkippedCount, annotateErr
				}
			}
			annotatedTagsCount += count
		}
		if len(lastTag) == 0 {
			break
		}
	}
	return annotatedTagsCount, totalSkippedCount, nil
}

// getManifestsToAnnotate gets all manifests that should be annotated according to the filter flag.
// Returns a pointer to a slice that contains the manifests that will be annotated and an error in case it occurred.
// Only manifests that would be annotated during a dry-run are printed here. If it's not a dry-run, there will
// be a print after a digest has been successfully annotated.
func getManifestsToAnnotate(ctx context.Context,
	acrClient api.AcrCLIClientInterface,
	orasClient api.ORASClientInterface,
	loginURL string,
	repoName string,
	filter *regexp2.Regexp,
	lastTag string, artifactType string, dryRun bool, includeLocked bool) ([]string, string, int, error) {

	resultTags, err := acrClient.GetAcrTags(ctx, repoName, "timedesc", lastTag)
	if err != nil {
		if resultTags != nil && resultTags.Response.Response != nil && resultTags.StatusCode == http.StatusNotFound {
			fmt.Printf("%s repository not found\n", repoName)
			return nil, "", 0, nil
		}
		return nil, "", 0, err
	}

	newLastTag := ""
	if resultTags != nil && resultTags.TagsAttributes != nil && len(*resultTags.TagsAttributes) > 0 {
		tags := *resultTags.TagsAttributes
		manifestsToAnnotate := []string{}
		skippedCount := 0
		for _, tag := range tags {
			matches, err := filter.MatchString(*tag.Name)
			if err != nil {
				// The only error that regexp2 will return is a timeout error
				return nil, "", 0, err
			}
			if !matches {
				// If a tag does not match the regex then it's not added to the list
				continue
			}

			// If a tag is changeable, then it is returned as a tag to annotate
			// With --include-locked flag, locked tags are also eligible for annotation
			if includeLocked || *tag.ChangeableAttributes.WriteEnabled {
				ref := fmt.Sprintf("%s/%s:%s", loginURL, repoName, *tag.Name)
				skip, err := orasClient.DiscoverLifecycleAnnotation(ctx, ref, artifactType)
				if err != nil {
					return nil, "", 0, err
				}
				if !skip {
					// Only print what would be annotated during a dry-run. Successfully annotated manifests
					// will be logged after the annotation.
					if dryRun {
						fmt.Printf("%s/%s:%s\n", loginURL, repoName, *tag.Name)
					}
					manifestsToAnnotate = append(manifestsToAnnotate, *tag.Digest)
				}
			} else {
				// Tag is locked and --include-locked is not set, skip it
				skippedCount++
			}
		}

		newLastTag = common.GetLastTagFromResponse(resultTags)
		return manifestsToAnnotate, newLastTag, skippedCount, nil
	}
	return nil, "", 0, nil
}

// annotateUntaggedManifests annotates all manifests that do not have any tags associated with them except the ones
// that are referenced by a multiarch manifest
func annotateUntaggedManifests(ctx context.Context,
	acrClient api.AcrCLIClientInterface,
	orasClient api.ORASClientInterface,
	poolSize int, loginURL string,
	repoName string, artifactType string,
	annotations []string,
	dryRun bool, includeLocked bool) (int, int, error) {
	if !dryRun {
		fmt.Printf("Annotating manifests for repository: %s\n", repoName)
	} else {
		fmt.Printf("Manifests for this repository would be annotated: %s\n", repoName)
	}

	// Contrary to getTagsToAnnotate, getManifests gets all the manifests at once.
	// This was done because if there is a manifest that has no tag but is referenced by a multiarch manifest that has tags then it
	// should not be annotated.
	manifestsToAnnotate, err := common.GetUntaggedManifests(ctx, poolSize, acrClient, repoName, true, nil, dryRun, includeLocked)
	if err != nil {
		return -1, 0, err
	}

	var annotator *worker.Annotator
	annotatedManifestsCount := 0
	if !dryRun {
		// In order to only have a limited amount of http requests, an annotator is used that will start goroutines to annotate manifests.
		annotator, err = worker.NewAnnotator(poolSize, orasClient, loginURL, repoName, artifactType, annotations)
		if err != nil {
			return -1, 0, err
		}
		manifestsCount, annotateErr := annotator.Annotate(ctx, manifestsToAnnotate)
		if annotateErr != nil {
			annotatedManifestsCount += manifestsCount
			return annotatedManifestsCount, 0, annotateErr
		}
		annotatedManifestsCount += manifestsCount
	} else {
		annotatedManifestsCount = len(manifestsToAnnotate)
	}

	// For untagged manifests, skipped count is 0 since locked manifests are handled by GetUntaggedManifests
	return annotatedManifestsCount, 0, nil

}
