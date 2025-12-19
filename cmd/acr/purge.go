// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package main

import (
	"context"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/Azure/acr-cli/acr"
	"github.com/Azure/acr-cli/cmd/repository"
	"github.com/Azure/acr-cli/internal/api"
	"github.com/Azure/acr-cli/internal/worker"
	"github.com/dlclark/regexp2"
	"github.com/spf13/cobra"
)

// The constants for this file are defined here.
const (
	newPurgeCmdLongMessage = `acr purge: untag old images and delete dangling manifests.`
	purgeExampleMessage    = `  - Delete all tags that are older than 1 day in the example.azurecr.io registry inside the hello-world repository
    	acr purge -r example --filter "hello-world:.*" --ago 1d

  - Delete all tags that are older than 7 days in the example.azurecr.io registry inside all repositories
	    acr purge -r example --filter ".*:.*" --ago 7d 

  - Delete all tags that are older than 7 days and begin with hello in the example.azurecr.io registry inside the hello-world repository
    	acr purge -r example --filter "hello-world:^hello.*" --ago 7d 

  - Delete all tags that are older than 7 days, begin with hello, keeping the latest 2 in example.azurecr.io registry inside the hello-world repository
    	acr purge -r example --filter "hello-world:^hello.*" --ago 7d --keep 2

  - Delete all tags that contain the word test in the tag name and are older than 5 days in the example.azurecr.io registry inside the hello-world 
    repository, after that, remove the dangling manifests in the same repository
	acr purge -r example --filter "hello-world:\w*test\w*" --ago 5d --untagged 

  - Delete all tags older than 1 day in the example.azurecr.io registry inside the hello-world repository using the credentials found in 
    the C://Users/docker/config.json path
	acr purge -r example --filter "hello-world:.*" --ago 1d --config C://Users/docker/config.json

  - Delete all tags older than 1 day in the example.azurecr.io registry inside the hello-world repository, with 4 purge tasks running concurrently
	acr purge -r example --filter "hello-world:.*" --ago 1d --concurrency 4

  - Delete all tags that are older than 7 days in the example.azurecr.io registry inside all repositories, with a page size of 50 repositories
	acr purge -r example --filter ".*:.*" --ago 7d --repository-page-size 50

  - Delete all tags that are older than 7 days in the example.azurecr.io registry inside all repositories, including locked manifests/tags
	acr purge -r example --filter ".*:.*" --ago 7d --include-locked
	`
	maxPoolSize = 32 // The max number of parallel delete requests recommended by ACR server
	headerLink  = "Link"
)

var (
	defaultPoolSize         = runtime.GOMAXPROCS(0)
	defaultRepoPageSize     = int32(100)
	repoPageSizeDescription = "Number of repositories queried at once"
	concurrencyDescription  = fmt.Sprintf("Number of concurrent purge tasks. Range: [1 - %d]", maxPoolSize)
)

// Default settings for regexp2
const (
	defaultRegexpMatchTimeoutSeconds int64 = 60
	maxAgoDurationYears              int   = 150 // Maximum duration in years for --ago flag to prevent overflow
)

// purgeParameters defines the parameters that the purge command uses (including the registry name, username and password).
type purgeParameters struct {
	*rootParameters
	ago           string
	keep          int
	filters       []string
	filterTimeout int64
	untagged      bool
	dryRun        bool
	includeLocked bool
	concurrency   int
	repoPageSize  int32
}

// newPurgeCmd defines the purge command.
func newPurgeCmd(rootParams *rootParameters) *cobra.Command {
	purgeParams := purgeParameters{rootParameters: rootParams}
	cmd := &cobra.Command{
		Use:     "purge",
		Short:   "Delete images from a registry.",
		Long:    newPurgeCmdLongMessage,
		Example: purgeExampleMessage,
		RunE: func(_ *cobra.Command, _ []string) error {
			// This context is used for all the http requests.
			ctx := context.Background()
			registryName, err := purgeParams.GetRegistryName()
			if err != nil {
				return err
			}
			loginURL := api.LoginURL(registryName)
			// An acrClient with authentication is generated, if the authentication cannot be resolved an error is returned.
			acrClient, err := api.GetAcrCLIClientWithAuth(loginURL, purgeParams.username, purgeParams.password, purgeParams.configs)
			if err != nil {
				return err
			}
			// A map is used to collect the regex tags for every repository.
			tagFilters, err := repository.CollectTagFilters(ctx, purgeParams.filters, acrClient.AutorestClient, purgeParams.filterTimeout, purgeParams.repoPageSize)
			if err != nil {
				return err
			}
			// A clarification message for --dry-run.
			if purgeParams.dryRun {
				fmt.Println("DRY RUN: The following output shows what WOULD be deleted if the purge command was executed. Nothing is deleted.")
			}

			// The number of concurrent requests will be ultimately limited by what repoParallelism is set to. This value
			// is at most maxPoolSize, and at least 1.
			repoParallelism := purgeParams.concurrency
			if repoParallelism <= 0 {
				repoParallelism = defaultPoolSize
				fmt.Printf("Specified concurrency value invalid. Set to default value: %d \n", defaultPoolSize)
			} else if repoParallelism > maxPoolSize {
				repoParallelism = maxPoolSize
				fmt.Printf("Specified concurrency value too large. Set to maximum value: %d \n", maxPoolSize)
			}

			deletedTagsCount, deletedManifestsCount, err := purge(ctx, acrClient, loginURL, repoParallelism, purgeParams.ago, purgeParams.keep, purgeParams.filterTimeout, purgeParams.untagged, tagFilters, purgeParams.dryRun, purgeParams.includeLocked)

			if err != nil {
				fmt.Printf("Failed to complete purge: %v \n", err)
			}

			// After all repos have been purged the summary is printed.
			if purgeParams.dryRun {
				fmt.Printf("\nNumber of tags to be deleted: %d\n", deletedTagsCount)
				fmt.Printf("Number of manifests to be deleted: %d\n", deletedManifestsCount)
			} else {
				fmt.Printf("\nNumber of deleted tags: %d\n", deletedTagsCount)
				fmt.Printf("Number of deleted manifests: %d\n", deletedManifestsCount)
			}

			return err
		},
	}

	cmd.Flags().BoolVar(&purgeParams.untagged, "untagged", false, "If the untagged flag is set all the manifests that do not have any tags associated to them will be also purged, except if they belong to a manifest list that contains at least one tag")
	cmd.Flags().BoolVar(&purgeParams.dryRun, "dry-run", false, "If the dry-run flag is set no manifest or tag will be deleted, the output would be the same as if they were deleted")
	cmd.Flags().BoolVar(&purgeParams.includeLocked, "include-locked", false, "If the include-locked flag is set, locked manifests and tags (where deleteEnabled or writeEnabled is false) will be unlocked before deletion")
	cmd.Flags().StringVar(&purgeParams.ago, "ago", "", "The tags and untagged manifests that were last updated before this duration will be deleted, the format is [number]d[string] where the first number represents an amount of days and the string is in a Go duration format (e.g. 2d3h6m selects images older than 2 days, 3 hours and 6 minutes). Maximum duration is capped at 150 years to prevent overflow")
	cmd.Flags().IntVar(&purgeParams.keep, "keep", 0, "Number of latest to-be-deleted tags to keep, use this when you want to keep at least x number of latest tags that could be deleted meeting all other filter criteria")
	cmd.Flags().StringArrayVarP(&purgeParams.filters, "filter", "f", nil, "Specify the repository and a regular expression filter for the tag name, if a tag matches the filter and is older than the duration specified in ago it will be deleted. Note: If backtracking is used in the regexp it's possible for the expression to run into an infinite loop. The default timeout is set to 1 minute for evaluation of any filter expression. Use the '--filter-timeout-seconds' option to set a different value.")
	cmd.Flags().StringArrayVarP(&purgeParams.configs, "config", "c", nil, "Authentication config paths (e.g. C://Users/docker/config.json)")
	cmd.Flags().Int64Var(&purgeParams.filterTimeout, "filter-timeout-seconds", defaultRegexpMatchTimeoutSeconds, "This limits the evaluation of the regex filter, and will return a timeout error if this duration is exceeded during a single evaluation. If written incorrectly a regexp filter with backtracking can result in an infinite loop.")
	cmd.Flags().IntVar(&purgeParams.concurrency, "concurrency", defaultPoolSize, concurrencyDescription)
	cmd.Flags().Int32Var(&purgeParams.repoPageSize, "repository-page-size", defaultRepoPageSize, repoPageSizeDescription)
	cmd.Flags().BoolP("help", "h", false, "Print usage")
	_ = cmd.MarkFlagRequired("filter")
	_ = cmd.MarkFlagRequired("ago")
	return cmd
}

func purge(ctx context.Context,
	acrClient api.AcrCLIClientInterface,
	loginURL string,
	repoParallelism int,
	tagDeletionSince string,
	tagsToKeep int,
	filterTimeout int64,
	removeUtaggedManifests bool,
	tagFilters map[string]string,
	dryRun bool,
	includeLocked bool) (deletedTagsCount int, deletedManifestsCount int, err error) {

	// Parse the duration once instead of for every repository
	agoDuration, err := parseDuration(tagDeletionSince)
	if err != nil {
		return 0, 0, err
	}

	// In order to print a summary of the deleted tags/manifests the counters get updated everytime a repo is purged.
	for repoName, tagRegex := range tagFilters {
		singleDeletedTagsCount, manifestToTagsCountMap, err := purgeTags(ctx, acrClient, repoParallelism, loginURL, repoName, agoDuration, tagRegex, tagsToKeep, filterTimeout, dryRun, includeLocked)
		if err != nil {
			return deletedTagsCount, deletedManifestsCount, fmt.Errorf("failed to purge tags: %w", err)
		}
		singleDeletedManifestsCount := 0
		// If the untagged flag is set then also manifests are deleted.
		if removeUtaggedManifests {
			singleDeletedManifestsCount, err = purgeDanglingManifests(ctx, acrClient, repoParallelism, loginURL, repoName, agoDuration, manifestToTagsCountMap, dryRun, includeLocked)
			if err != nil {
				return deletedTagsCount, deletedManifestsCount, fmt.Errorf("failed to purge manifests: %w", err)
			}
		}
		// After every repository is purged the counters are updated.
		deletedTagsCount += singleDeletedTagsCount
		deletedManifestsCount += singleDeletedManifestsCount
	}

	return deletedTagsCount, deletedManifestsCount, nil

}

// purgeTags deletes all tags that are older than the agoDuration value and that match the tagFilter string.
func purgeTags(ctx context.Context, acrClient api.AcrCLIClientInterface, repoParallelism int, loginURL string, repoName string, agoDuration time.Duration, tagFilter string, keep int, regexpMatchTimeoutSeconds int64, dryRun bool, includeLocked bool) (int, map[string]int, error) {
	if dryRun {
		fmt.Printf("Would delete tags for repository: %s\n", repoName)
	} else {
		fmt.Printf("Deleting tags for repository: %s\n", repoName)
	}
	manifestToTagsCountMap := make(map[string]int) // This map is used to keep track of how many tags would have been deleted per manifest.
	timeToCompare := time.Now().UTC()
	// Since the parseDuration function returns a negative duration, it is added to the current duration in order to be able to easily compare
	// with the LastUpdatedTime attribute a tag has.
	timeToCompare = timeToCompare.Add(agoDuration)

	tagRegex, err := repository.BuildRegexFilter(tagFilter, regexpMatchTimeoutSeconds)
	if err != nil {
		return -1, manifestToTagsCountMap, fmt.Errorf("failed to build Regex %s with error: %w", tagRegex, err)
	}
	lastTag := ""
	skippedTagsCount := 0
	deletedTagsCount := 0
	// In order to only have a limited amount of http requests, a purger is used that will start goroutines to delete tags.
	purger := worker.NewPurger(repoParallelism, acrClient, loginURL, repoName, includeLocked)

	// GetTagsToDelete will return an empty lastTag when there are no more tags.
	for {
		tagsToDelete, newLastTag, newSkippedTagsCount, err := getTagsToDelete(ctx, acrClient, repoName, tagRegex, timeToCompare, lastTag, keep, skippedTagsCount, includeLocked)
		if err != nil {
			return -1, manifestToTagsCountMap, err
		}
		lastTag = newLastTag
		skippedTagsCount = newSkippedTagsCount
		if len(tagsToDelete) > 0 {
			for _, tag := range tagsToDelete {
				manifestToTagsCountMap[*tag.Digest]++
				if dryRun {
					fmt.Printf("Would delete: %s/%s:%s\n", loginURL, repoName, *tag.Name)
				}
			}

			if dryRun {
				deletedTagsCount += len(tagsToDelete)
				if len(lastTag) == 0 {
					break
				}
				continue // If dryRun is set to true then no tags will be deleted, but the count is updated.
			}

			count, purgeErr := purger.PurgeTags(ctx, tagsToDelete)
			if purgeErr != nil {
				return -1, manifestToTagsCountMap, purgeErr
			}
			deletedTagsCount += count
		}
		if len(lastTag) == 0 {
			break
		}
	}

	return deletedTagsCount, manifestToTagsCountMap, nil
}

// parseDuration analog to time.ParseDuration() but with days added.
func parseDuration(ago string) (time.Duration, error) {
	var days int
	var durationString string
	// The supported format is %d%s where the string is a valid go duration string.
	if strings.Contains(ago, "d") {
		if _, err := fmt.Sscanf(ago, "%dd%s", &days, &durationString); err != nil {
			_, _ = fmt.Sscanf(ago, "%dd", &days)
			durationString = ""
		}
	} else {
		days = 0
		if _, err := fmt.Sscanf(ago, "%s", &durationString); err != nil {
			return time.Duration(0), err
		}
	}
	// Cap at maxAgoDurationYears to prevent overflow
	const maxDays = maxAgoDurationYears * 365
	originalDays := days
	capped := false
	if days > maxDays {
		days = maxDays
		capped = true
		fmt.Printf("Warning: ago value exceeds maximum duration of %d years, capping to %d years\n", maxAgoDurationYears, maxAgoDurationYears)
	}
	// The number of days gets converted to hours.
	duration := time.Duration(days) * 24 * time.Hour
	if len(durationString) > 0 {
		agoDuration, err := time.ParseDuration(durationString)
		if err != nil {
			// Check if it's an overflow error from time.ParseDuration
			if strings.Contains(err.Error(), "invalid duration") || strings.Contains(err.Error(), "overflow") {
				// If days were already capped, just use that and ignore the overflow portion
				if capped {
					return (-1 * duration), nil
				}
				// Cap at max duration and continue
				agoDuration = time.Duration(maxDays) * 24 * time.Hour
				fmt.Printf("Warning: ago value exceeds maximum duration of %d years, capping to %d years\n", maxAgoDurationYears, maxAgoDurationYears)
			} else {
				return time.Duration(0), err
			}
		}
		// Cap the additional duration to prevent overflow when adding
		maxDuration := time.Duration(maxDays) * 24 * time.Hour
		if agoDuration > maxDuration {
			agoDuration = maxDuration
			if originalDays <= maxDays && !capped {
				// Only print warning if we haven't already printed one for days
				fmt.Printf("Warning: ago value exceeds maximum duration of %d years, capping to %d years\n", maxAgoDurationYears, maxAgoDurationYears)
			}
		}
		// Make sure the combined duration doesn't exceed max
		duration = duration + agoDuration
		if duration > maxDuration {
			duration = maxDuration
		}
	}
	return (-1 * duration), nil
}

// getTagsToDelete gets all tags that should be deleted according to the ago flag and the filter flag, this will at most return 100 tags,
// returns a pointer to a slice that contains the tags that will be deleted, the last tag obtained through the AcrListTags function
// and an error in case it occurred, the fourth return value contains a map that is used to determine how many tags a manifest has
func getTagsToDelete(ctx context.Context,
	acrClient api.AcrCLIClientInterface,
	repoName string,
	filter *regexp2.Regexp,
	timeToCompare time.Time,
	lastTag string,
	keep int,
	skippedTagsCount int,
	includeLocked bool) ([]acr.TagAttributesBase, string, int, error) {

	var matches bool
	var lastUpdateTime time.Time
	resultTags, err := acrClient.GetAcrTags(ctx, repoName, "timedesc", lastTag)
	if err != nil {
		if resultTags != nil && resultTags.Response.Response != nil && resultTags.StatusCode == http.StatusNotFound {
			fmt.Printf("%s repository not found\n", repoName)
			return nil, "", skippedTagsCount, nil
		}
		// An empty lastTag string is returned so there will not be any tag purged.
		return nil, "", skippedTagsCount, err
	}
	newLastTag := ""
	if resultTags != nil && resultTags.TagsAttributes != nil && len(*resultTags.TagsAttributes) > 0 {
		tags := *resultTags.TagsAttributes
		tagsEligibleForDeletion := []acr.TagAttributesBase{}
		for _, tag := range tags {
			matches, err = filter.MatchString(*tag.Name)
			if err != nil {
				// The only error that regexp2 will return is a timeout error
				return nil, "", skippedTagsCount, err
			}
			if !matches {
				// If a tag does not match the regex then it not added to the list no matter the LastUpdateTime
				continue
			}
			lastUpdateTime, err = time.Parse(time.RFC3339Nano, *tag.LastUpdateTime)
			if err != nil {
				return nil, "", skippedTagsCount, err
			}
			// If a tag did match the regex filter, is older than the specified duration and can be deleted then it is returned
			// as a tag to delete. With --include-locked flag, locked tags are also eligible for deletion.
			if lastUpdateTime.Before(timeToCompare) {
				if includeLocked || (*(*tag.ChangeableAttributes).DeleteEnabled && *(*tag.ChangeableAttributes).WriteEnabled) {
					tagsEligibleForDeletion = append(tagsEligibleForDeletion, tag)
				}
			}
		}

		newLastTag = repository.GetLastTagFromResponse(resultTags)
		// No more tags to keep
		if keep == 0 || skippedTagsCount == keep {
			return tagsEligibleForDeletion, newLastTag, skippedTagsCount, nil
		}

		tagsToDelete := []acr.TagAttributesBase{}
		for _, tag := range tagsEligibleForDeletion {
			// Keep at least the configured number of tags
			if skippedTagsCount < keep {
				skippedTagsCount++
			} else {
				tagsToDelete = append(tagsToDelete, tag)
			}
		}
		return tagsToDelete, newLastTag, skippedTagsCount, nil
	}
	// In case there are no more tags return empty string as lastTag so that the purgeTags function stops
	return nil, "", skippedTagsCount, nil
}

// purgeDanglingManifests deletes all manifests that do not have any tags associated with them.
// except the ones that are referenced by a multiarch manifest or that have subject.
func purgeDanglingManifests(ctx context.Context, acrClient api.AcrCLIClientInterface, repoParallelism int, loginURL string, repoName string, agoDuration time.Duration, manifestToTagsCountMap map[string]int, dryRun bool, includeLocked bool) (int, error) {
	if dryRun {
		fmt.Printf("Would delete manifests for repository: %s\n", repoName)
	} else {
		fmt.Printf("Deleting manifests for repository: %s\n", repoName)
	}
	timeToCompare := time.Now().UTC().Add(agoDuration)
	// Contrary to getTagsToDelete, getManifestsToDelete gets all the Manifests at once, this was done because if there is a manifest that has no
	// tag but is referenced by a multiarch manifest that has tags then it should not be deleted. Or if a manifest has no tag, but it has subject,
	// then it should not be deleted.
	manifestsToDelete, err := repository.GetUntaggedManifests(ctx, repoParallelism, acrClient, repoName, false, manifestToTagsCountMap, dryRun, includeLocked, &timeToCompare)
	if err != nil {
		return -1, err
	}

	// If dryRun is set to true then no manifests will be deleted, but the number of manifests that would be deleted is returned. Additionally,
	// the manifests that would be deleted are printed to the console. We also need to account for the manifests that would be deleted from the tag
	// filtering first as that would influence the untagged manifests that would be deleted.
	if dryRun {
		for _, manifest := range manifestsToDelete {
			fmt.Printf("Would delete: %s/%s@%s\n", loginURL, repoName, *manifest.Digest)
		}
		return len(manifestsToDelete), nil
	}
	// In order to only have a limited amount of http requests, a purger is used that will start goroutines to delete manifests.
	purger := worker.NewPurger(repoParallelism, acrClient, loginURL, repoName, includeLocked)
	deletedManifestsCount, purgeErr := purger.PurgeManifests(ctx, manifestsToDelete)
	if purgeErr != nil {
		return -1, purgeErr
	}
	return deletedManifestsCount, nil
}
