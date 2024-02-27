// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package main

import (
	// "context"
	"context"
	"fmt"
	"net/http"

	"github.com/Azure/acr-cli/acr"
	"github.com/Azure/acr-cli/cmd/api"
	"github.com/dlclark/regexp2"
	"github.com/spf13/cobra"
)

// The constants for the file are defined here
const (
	newAnnotateCmdLongMessage = `acr annotate: annotate images and dangling manifests.`
	// TODO: write examples of the annotate command for different scenarios (see purge examples)
	annotateExampleMessage = ``
)

var (
	annotatedConcurrencyDescription = fmt.Sprintf("Number of concurrent annotate tasks. Range: [1 - %d]", maxPoolSize)
)

// // Default settings for regexp2
// const (
// 	defaultRegexpOptions			regext2.RegexOptions = regexp2.RE2 // This option will turn on compatibility mode so that it uses the group rules in regexp

// )

// annotateParameters defines the parameters that the annotate command uses (including the registry name, username, and password)
type annotateParameters struct {
	*rootParameters
	filters       []string
	filterTimeout uint64
	artifactType  string
	annotation    string
	untagged      bool
	dryRun        bool
	concurrency   int
}

// newAnnotateCmd defines the annotate command
func newAnnotateCmd(rootParams *rootParameters) *cobra.Command {
	annotateParams := annotateParameters{rootParameters: rootParams}
	cmd := &cobra.Command{
		Use:     "annotate",
		Short:   "Annotate images in a registry",
		Long:    newAnnotateCmdLongMessage,
		Example: annotateExampleMessage,
		RunE: func(cmd *cobra.Command, args []string) error {
			// This context is used for all the http requests
			// ctx := context.Background()
			// registryName, err := annotateParams.GetRegistryName()
			// if err != nil {
			// 	return err
			// }
			// loginURL := api.LoginURL(registryName)
			// // An acrClient with authentication is generated, if the authentication cannot be resolved an error is returned.
			// acrClient, err := api.GetAcrCLIClientWithAuth(loginURL, annotateParams.username, annotateParams.password, annotateParams.configs)
			// if err != nil {
			// 	return err
			// }
			// // A map is used to collect the regex tags for every repository.
			// tagFilters, err := collectTagFilters(ctx, annotateParams.filters, acrClient.AutorestClient, annotateParams.filterTimeout)
			// if err != nil {
			// 	return err
			// }
			// // A clarification message for --dry-run.
			// if annotateParams.dryRun {
			// 	fmt.Println("DRY RUN: The following output shows what WOULD be annotated if the annotate command was executed. Nothing is annotated.")
			// }
			return nil
		},
	}

	cmd.Flags().StringArrayVarP(&annotateParams.filters, "filter", "f", nil, "Specify the repository and a regular expression filter for the tag name. If a tag matches the filter, it will be annotated. Note: If backtracking is used in the regexp it's possible for the expression to run into an infinite loop. The default timeout is set to 1 minute for evaluation of any filter expression. Use the '--filter-timeout-seconds' option to set a different value")
	cmd.Flags().Uint64Var(&annotateParams.filterTimeout, "filter-timeout-seconds", defaultRegexpMatchTimeoutSeconds, "This limits the evaluation of the regex filter, and will return a timeout error if this duration is exceeded during a single evaluation. If written incorrectly a regexp filter with backtracking can result in an infinite loop")
	cmd.Flags().StringVar(&annotateParams.artifactType, "artifact-type", "", "The configurable artifact type for an organization")
	cmd.Flags().StringVar(&annotateParams.annotation, "annotation", "", "The configurable annotation key value that can be specified one or more times")
	cmd.Flags().BoolVar(&annotateParams.untagged, "untagged", false, "If the untagged flag is set, all the manifests that do not have any tags associated to them will also be annotated, except if they belong to a manifest list that contains at least one tag")
	cmd.Flags().BoolVar(&annotateParams.dryRun, "dry-run", false, "If the dry-run flag is set, no manifest or tag will be annotated. The output would be the same as if they were annotated")
	cmd.Flags().IntVar(&annotateParams.concurrency, "concurrency", defaultPoolSize, annotatedConcurrencyDescription)
	cmd.Flags().BoolP("help", "h", false, "Print usage")
	cmd.MarkFlagRequired("filter")
	cmd.MarkFlagRequired("artifact-type")
	cmd.MarkFlagRequired("annotation")
	return cmd
}

// annotateTags annotates all tags that match the tagFilter string.

// getTagsToAnnotate gets all tags that should be annotated according to the filter flag. This will at most return 100 flags.
// Returns a pointer to a slice that contains the tags that will be annotated and an error in case it occurred.
func getTagsToAnnotate(ctx context.Context,
	acrClient api.AcrCLIClientInterface,
	repoName string,
	filter *regexp2.Regexp, lastTag string) (*[]acr.TagAttributesBase, error) {

	var matches bool
	resultTags, err := acrClient.GetAcrTags(ctx, repoName, "timedesc", lastTag)
	if err != nil {
		if resultTags != nil && resultTags.Response.Response != nil && resultTags.StatusCode == http.StatusNotFound {
			fmt.Printf("%s repository not found\n", repoName)
			return nil, nil
		}
		return nil, err
	}
	// newLastTag := ""
	if resultTags != nil && resultTags.TagsAttributes != nil && len(*resultTags.TagsAttributes) > 0 {
		tags := *resultTags.TagsAttributes
		tagsToAnnotate := []acr.TagAttributesBase{}
		for _, tag := range tags {
			matches, err = filter.MatchString(*tag.Name)
			if err != nil {
				// The only error that regexp2 will return is a timeout error
				return nil, err
			}
			if !matches {
				// If a tag does not match the regex then it's not added to the list
				continue
			}
			tagsToAnnotate = append(tagsToAnnotate, tag)
		}

		// newLastTag = getLastTagFromResponse(resultTags)
		return &tagsToAnnotate, nil
	}
	return nil, nil
}
