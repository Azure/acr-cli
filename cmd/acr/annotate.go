// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/Azure/acr-cli/acr"
	"github.com/Azure/acr-cli/cmd/api"
	"github.com/Azure/acr-cli/cmd/worker"
	"github.com/dlclark/regexp2"
	"github.com/spf13/cobra"
)

// The constants for the file are defined here
const (
	newAnnotateCmdLongMessage = `acr annotate: annotate images and dangling manifests.`
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
	filterTimeout uint64
	artifactType  string
	annotations   []string
	untagged      bool
	dryRun        bool
	concurrency   int
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
			tagFilters, err := collectTagFilters(ctx, annotateParams.filters, acrClient.AutorestClient, annotateParams.filterTimeout)
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

			poolSize := annotateParams.concurrency
			if poolSize <= 0 {
				poolSize = defaultPoolSize
				fmt.Printf("Specified concurrency value invalid. Set to default value: %d \n", defaultPoolSize)
			} else if poolSize > maxPoolSize {
				poolSize = maxPoolSize
				fmt.Printf("Specified concurrency value too large. Set to maximum value: %d \n", maxPoolSize)
			}
			for repoName, tagRegex := range tagFilters {
				singleAnnotatedTagsCount, err := annotateTags(ctx, acrClient, orasClient, poolSize, loginURL, repoName, annotateParams.artifactType, annotateParams.annotations, tagRegex, annotateParams.filterTimeout, annotateParams.dryRun)
				if err != nil {
					return fmt.Errorf("failed to annotate tags: %w", err)
				}

				singleAnnotatedManifestsCount := 0
				// If the untagged flag is set, then manifests with no tags are also annotated..
				if annotateParams.untagged {
					singleAnnotatedManifestsCount, err = annotateDanglingManifests(ctx, acrClient, orasClient, poolSize, loginURL, repoName, annotateParams.artifactType, annotateParams.annotations, annotateParams.dryRun)
					if err != nil {
						return fmt.Errorf("failed to annotate manifests: %w", err)
					}
				}

				// After every repository is annotated, the counters are updated
				annotatedTagsCount += singleAnnotatedTagsCount
				annotatedManifestsCount += singleAnnotatedManifestsCount
			}

			// After all repos have been purged, the summary is printed
			if annotateParams.dryRun {
				fmt.Printf("\nNumber of tags to be annotated: %d", annotatedTagsCount)
				fmt.Printf("\nNumber of manifests to be annotated: %d\n", annotatedManifestsCount)
			} else {
				fmt.Printf("\nNumber of annotated tags: %d", annotatedTagsCount)
				fmt.Printf("\nNumber of annotated manifests: %d\n", annotatedManifestsCount)
			}
			return nil
		},
	}

	cmd.Flags().StringArrayVarP(&annotateParams.filters, "filter", "f", nil, "Specify the repository and a regular expression filter for the tag name. If a tag matches the filter, it will be annotated. Note: If backtracking is used in the regexp it's possible for the expression to run into an infinite loop. The default timeout is set to 1 minute for evaluation of any filter expression. Use the '--filter-timeout-seconds' option to set a different value")
	cmd.Flags().Uint64Var(&annotateParams.filterTimeout, "filter-timeout-seconds", defaultRegexpMatchTimeoutSeconds, "This limits the evaluation of the regex filter, and will return a timeout error if this duration is exceeded during a single evaluation. If written incorrectly a regexp filter with backtracking can result in an infinite loop")
	cmd.Flags().StringVar(&annotateParams.artifactType, "artifact-type", "", "The configurable artifact type for an organization")
	cmd.Flags().StringSliceVarP(&annotateParams.annotations, "annotations", "a", []string{}, "The configurable annotation key value that can be specified one or more times")
	cmd.Flags().BoolVar(&annotateParams.untagged, "untagged", false, "If the untagged flag is set, all the manifests that do not have any tags associated to them will also be annotated, except if they belong to a manifest list that contains at least one tag")
	cmd.Flags().BoolVar(&annotateParams.dryRun, "dry-run", false, "If the dry-run flag is set, no manifest or tag will be annotated. The output would be the same as if they were annotated")
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
	regexpMatchTimeoutSeconds uint64,
	dryRun bool) (int, error) {

	if !dryRun {
		fmt.Printf("Annotating tags for repository: %s\n", repoName)
	} else {
		fmt.Printf("Tags for this repository would be annotated: %s\n", repoName)
	}

	// In order to keep track if a manifest would get annotated during a dry-run, a map is defined that as a key has the manifest
	// digest and as the value, the number of tags (referencing said manifest) that were annotated.
	annotatedTags := map[string]int{}

	tagRegex, err := buildRegexFilter(tagFilter, regexpMatchTimeoutSeconds)
	if err != nil {
		return -1, err
	}

	lastTag := ""
	annotatedTagsCount := 0

	var annotator *worker.Annotator
	if !dryRun {
		// In order to only have a limited amount of http requests, an annotator is used that will start goroutines to annotate tags.
		annotator, err = worker.NewAnnotator(poolSize, orasClient, loginURL, repoName, artifactType, annotations)
		if err != nil {
			return -1, err
		}
	}

	for {
		// GetTagsToAnnotate will return an empty lastTag when there are no more tags.
		tagsToAnnotate, newLastTag, err := getTagsToAnnotate(ctx, acrClient, repoName, tagRegex, lastTag)
		if err != nil {
			return -1, err
		}
		lastTag = newLastTag
		if tagsToAnnotate != nil {
			if !dryRun {
				count, annotateErr := annotator.AnnotateTags(ctx, tagsToAnnotate)
				if annotateErr != nil {
					return -1, annotateErr
				}
				annotatedTagsCount += count
			} else {
				for _, tag := range *tagsToAnnotate {
					// For every tag that would be annotated, first check if it exists in the map. If it doesn't, add a new key
					// with value 1 and if it does, just add 1 to the existent value.
					annotatedTags[*tag.Digest]++
					fmt.Printf("%s/%s:%s\n", loginURL, repoName, *tag.Name)
					annotatedTagsCount++
				}
			}
		}
		if len(lastTag) == 0 {
			break
		}

	}

	return annotatedTagsCount, nil
}

// getTagsToAnnotate gets all tags that should be annotated according to the filter flag. This will at most return 100 flags.
// Returns a pointer to a slice that contains the tags that will be annotated and an error in case it occurred.
func getTagsToAnnotate(ctx context.Context,
	acrClient api.AcrCLIClientInterface,
	repoName string,
	filter *regexp2.Regexp, lastTag string) (*[]acr.TagAttributesBase, string, error) {

	var matches bool
	resultTags, err := acrClient.GetAcrTags(ctx, repoName, "timedesc", lastTag)
	if err != nil {
		if resultTags != nil && resultTags.Response.Response != nil && resultTags.StatusCode == http.StatusNotFound {
			fmt.Printf("%s repository not found\n", repoName)
			return nil, "", nil
		}
		return nil, "", err
	}

	newLastTag := ""
	if resultTags != nil && resultTags.TagsAttributes != nil && len(*resultTags.TagsAttributes) > 0 {
		tags := *resultTags.TagsAttributes
		tagsToAnnotate := []acr.TagAttributesBase{}
		for _, tag := range tags {
			matches, err = filter.MatchString(*tag.Name)
			if err != nil {
				// The only error that regexp2 will return is a timeout error
				return nil, "", err
			}
			if !matches {
				// If a tag does not match the regex then it's not added to the list
				continue
			}
			// If a tag is changable, then it is returned as a tag to annotate
			if *tag.ChangeableAttributes.WriteEnabled {
				tagsToAnnotate = append(tagsToAnnotate, tag)
			}
		}

		newLastTag = getLastTagFromResponse(resultTags)
		return &tagsToAnnotate, newLastTag, nil
	}
	return nil, "", nil
}

// annotateDanglingManifests annotates all manifests that do not have any tags associated with them except the ones
// that are referenced by a multiarch manifest
func annotateDanglingManifests(ctx context.Context, acrClient api.AcrCLIClientInterface, orasClient api.ORASClientInterface, poolSize int, loginURL string, repoName string, artifactType string, annotations []string, dryRun bool) (int, error) {
	if !dryRun {
		fmt.Printf("Annotating manifests for repository: %s\n", repoName)
	} else {
		fmt.Printf("Manifests for this repository would be annotated: %s\n", repoName)
	}

	// Contrary to getTagsToAnnotate, getManifestsToAnnotate gets all the manifests at once.
	// This was done because if there is a manifest that has no tag but is referenced by a multiarch manifest that has tags then it
	// should not be annotated.
	manifestsToAnnotate, err := getManifests(ctx, acrClient, loginURL, repoName, dryRun, false)
	if err != nil {
		return -1, err
	}

	var annotator *worker.Annotator
	annotatedManifestsCount := 0
	if !dryRun {
		// In order to only have a limited amount of http requests, an annotator is used that will start goroutines to annotate manifests.
		annotator, err = worker.NewAnnotator(poolSize, orasClient, loginURL, repoName, artifactType, annotations)
		if err != nil {
			return -1, err
		}
		manifestsCount, annotateErr := annotator.AnnotateManifests(ctx, manifestsToAnnotate)
		if annotateErr != nil {
			return manifestsCount, annotateErr
		}
		annotatedManifestsCount += manifestsCount
	} else {
		annotatedManifestsCount = len(*manifestsToAnnotate)
	}

	return annotatedManifestsCount, nil

}

// getManifestsToAnnotate gets all the manifests that should be annotated under the scenario that they do not have any tag
// and do not form part of a manifest list that has tags referencing it.
// func getManifestsToAnnotate(ctx context.Context, acrClient api.AcrCLIClientInterface, loginURL string, repoName string, dryRun bool) (*[]acr.ManifestAttributesBase, error) {
// 	lastManifestDigest := ""
// 	var manifestsToAnnotate []acr.ManifestAttributesBase
// 	resultManifests, err := acrClient.GetAcrManifests(ctx, repoName, "", lastManifestDigest)
// 	if err != nil {
// 		if resultManifests != nil && resultManifests.Response.Response != nil && resultManifests.StatusCode == http.StatusNotFound {
// 			fmt.Printf("%s repository not found\n", repoName)
// 			return &manifestsToAnnotate, nil
// 		}
// 		return nil, err
// 	}
// 	// This will act as a set. If a key is present then it should not be annotated because it is referenced by a multiarch manifest.
// 	doNotAnnotate := set.New[string]()
// 	var candidatesToAnnotate []acr.ManifestAttributesBase
// 	for resultManifests != nil && resultManifests.ManifestsAttributes != nil {
// 		manifests := *resultManifests.ManifestsAttributes
// 		for _, manifest := range manifests {
// 			if manifest.Tags != nil {
// 				// If a manifest has tags and its media type supports multiarch manifests, we will
// 				// iterate all its dependent manifests and mark them as not to be annotated.
// 				err = doNotAnnotateDependantManifests(ctx, manifest, doNotAnnotate, acrClient, repoName)
// 				if err != nil {
// 					return nil, err
// 				}
// 			} else {
// 				candidatesToAnnotate = append(candidatesToAnnotate, manifest)
// 			}
// 		}

// 		// Get the last manifest digest from the last manifest from manifests
// 		lastManifestDigest = *manifests[len(manifests)-1].Digest
// 		// Use this new digest to find the next batch of manifests
// 		resultManifests, err = acrClient.GetAcrManifests(ctx, repoName, "", lastManifestDigest)
// 		if err != nil {
// 			return nil, err
// 		}
// 	}

// 	// Remove all manifests that should not be annotated
// 	for i := 0; i < len(candidatesToAnnotate); i++ {
// 		if !doNotAnnotate.Contains(*candidatesToAnnotate[i].Digest) {
// 			// if a manifest has no tags, is not part of a manifest list and can be annotated then it is added to the
// 			// manifestsToAnnotate array.
// 			if *(*candidatesToAnnotate[i].ChangeableAttributes).WriteEnabled {
// 				manifestsToAnnotate = append(manifestsToAnnotate, candidatesToAnnotate[i])
// 				if dryRun {
// 					fmt.Printf("%s/%s@%s\n", loginURL, repoName, *candidatesToAnnotate[i].Digest)
// 				}
// 			}
// 		}
// 	}

// 	return &manifestsToAnnotate, nil
// }

// doNotAnnotateDependantManifests adds the dependant manifest to doNotAnnotate
// if the referred manifest has tags.
// func doNotAnnotateDependantManifests(ctx context.Context, manifest acr.ManifestAttributesBase, doNotAnnotate set.Set[string], acrClient api.AcrCLIClientInterface, repoName string) error {
// 	switch *manifest.MediaType {
// 	case mediaTypeDockerManifestList, v1.MediaTypeImageIndex:
// 		var manifestBytes []byte
// 		manifestBytes, err := acrClient.GetManifest(ctx, repoName, *manifest.Digest)
// 		if err != nil {
// 			return err
// 		}
// 		// this struct defines a customized struct for manifests
// 		// which is used to parse the content of a multiarch manifest
// 		mam := struct {
// 			Manifests []v1.Descriptor `json:"manifests"`
// 		}{}

// 		if err = json.Unmarshal(manifestBytes, &mam); err != nil {
// 			return err
// 		}
// 		for _, dependentManifest := range mam.Manifests {
// 			doNotAnnotate.Add(dependentManifest.Digest.String())
// 		}
// 	}
// 	return nil
// }
