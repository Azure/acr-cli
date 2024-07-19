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
	"github.com/Azure/acr-cli/cmd/common"
	"github.com/Azure/acr-cli/internal/api"
	"github.com/Azure/acr-cli/internal/container/set"
	"github.com/Azure/acr-cli/internal/worker"
	"github.com/dlclark/regexp2"
	"github.com/pkg/errors"
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
	`
	maxPoolSize = 32 // The max number of parallel delete requests recommended by ACR server
	headerLink  = "Link"
)

var (
	defaultPoolSize        = runtime.GOMAXPROCS(0)
	concurrencyDescription = fmt.Sprintf("Number of concurrent purge tasks. Range: [1 - %d]", maxPoolSize)
)

// Default settings for regexp2
const (
	defaultRegexpMatchTimeoutSeconds uint64 = 60
)

// purgeParameters defines the parameters that the purge command uses (including the registry name, username and password).
type purgeParameters struct {
	*rootParameters
	ago           string
	keep          int
	filters       []string
	filterTimeout uint64
	untagged      bool
	dryRun        bool
	concurrency   int
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
			tagFilters, err := common.CollectTagFilters(ctx, purgeParams.filters, acrClient.AutorestClient, purgeParams.filterTimeout)
			if err != nil {
				return err
			}
			// A clarification message for --dry-run.
			if purgeParams.dryRun {
				fmt.Println("DRY RUN: The following output shows what WOULD be deleted if the purge command was executed. Nothing is deleted.")
			}
			// In order to print a summary of the deleted tags/manifests the counters get updated everytime a repo is purged.
			deletedTagsCount := 0
			deletedManifestsCount := 0
			for repoName, tagRegex := range tagFilters {
				if !purgeParams.dryRun {
					poolSize := purgeParams.concurrency
					if poolSize <= 0 {
						poolSize = defaultPoolSize
						fmt.Printf("Specified concurrency value invalid. Set to default value: %d \n", defaultPoolSize)
					} else if poolSize > maxPoolSize {
						poolSize = maxPoolSize
						fmt.Printf("Specified concurrency value too large. Set to maximum value: %d \n", maxPoolSize)
					}

					singleDeletedTagsCount, err := purgeTags(ctx, acrClient, poolSize, loginURL, repoName, purgeParams.ago, tagRegex, purgeParams.keep, purgeParams.filterTimeout)
					if err != nil {
						return errors.Wrap(err, "failed to purge tags")
					}
					singleDeletedManifestsCount := 0
					// If the untagged flag is set then also manifests are deleted.
					if purgeParams.untagged {
						singleDeletedManifestsCount, err = purgeDanglingManifests(ctx, acrClient, poolSize, loginURL, repoName)
						if err != nil {
							return errors.Wrap(err, "failed to purge manifests")
						}
					}
					// After every repository is purged the counters are updated.
					deletedTagsCount += singleDeletedTagsCount
					deletedManifestsCount += singleDeletedManifestsCount
				} else {
					// No tag or manifest will be deleted but the counters still will be updated.
					singleDeletedTagsCount, singleDeletedManifestsCount, err := dryRunPurge(ctx, acrClient, loginURL, repoName, purgeParams.ago, tagRegex, purgeParams.untagged, purgeParams.keep, purgeParams.filterTimeout)
					if err != nil {
						return errors.Wrap(err, "failed to dry-run purge")
					}
					deletedTagsCount += singleDeletedTagsCount
					deletedManifestsCount += singleDeletedManifestsCount
				}
			}
			// After all repos have been purged the summary is printed.
			if purgeParams.dryRun {
				fmt.Printf("\nNumber of tags to be deleted: %d\n", deletedTagsCount)
				fmt.Printf("Number of manifests to be deleted: %d\n", deletedManifestsCount)
			} else {
				fmt.Printf("\nNumber of deleted tags: %d\n", deletedTagsCount)
				fmt.Printf("Number of deleted manifests: %d\n", deletedManifestsCount)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&purgeParams.untagged, "untagged", false, "If the untagged flag is set all the manifests that do not have any tags associated to them will be also purged, except if they belong to a manifest list that contains at least one tag")
	cmd.Flags().BoolVar(&purgeParams.dryRun, "dry-run", false, "If the dry-run flag is set no manifest or tag will be deleted, the output would be the same as if they were deleted")
	cmd.Flags().StringVar(&purgeParams.ago, "ago", "", "The tags that were last updated before this duration will be deleted, the format is [number]d[string] where the first number represents an amount of days and the string is in a Go duration format (e.g. 2d3h6m selects images older than 2 days, 3 hours and 6 minutes)")
	cmd.Flags().IntVar(&purgeParams.keep, "keep", 0, "Number of latest to-be-deleted tags to keep, use this when you want to keep at least x number of latest tags that could be deleted meeting all other filter criteria")
	cmd.Flags().StringArrayVarP(&purgeParams.filters, "filter", "f", nil, "Specify the repository and a regular expression filter for the tag name, if a tag matches the filter and is older than the duration specified in ago it will be deleted. Note: If backtracking is used in the regexp it's possible for the expression to run into an infinite loop. The default timeout is set to 1 minute for evaluation of any filter expression. Use the '--filter-timeout-seconds' option to set a different value.")
	cmd.Flags().StringArrayVarP(&purgeParams.configs, "config", "c", nil, "Authentication config paths (e.g. C://Users/docker/config.json)")
	cmd.Flags().Uint64Var(&purgeParams.filterTimeout, "filter-timeout-seconds", defaultRegexpMatchTimeoutSeconds, "This limits the evaluation of the regex filter, and will return a timeout error if this duration is exceeded during a single evaluation. If written incorrectly a regexp filter with backtracking can result in an infinite loop.")
	cmd.Flags().IntVar(&purgeParams.concurrency, "concurrency", defaultPoolSize, concurrencyDescription)
	cmd.Flags().BoolP("help", "h", false, "Print usage")
	cmd.MarkFlagRequired("filter")
	cmd.MarkFlagRequired("ago")
	return cmd
}

// purgeTags deletes all tags that are older than the ago value and that match the tagFilter string.
func purgeTags(ctx context.Context, acrClient api.AcrCLIClientInterface, poolSize int, loginURL string, repoName string, ago string, tagFilter string, keep int, regexpMatchTimeoutSeconds uint64) (int, error) {
	fmt.Printf("Deleting tags for repository: %s\n", repoName)
	agoDuration, err := parseDuration(ago)
	if err != nil {
		return -1, err
	}
	timeToCompare := time.Now().UTC()
	// Since the parseDuration function returns a negative duration, it is added to the current duration in order to be able to easily compare
	// with the LastUpdatedTime attribute a tag has.
	timeToCompare = timeToCompare.Add(agoDuration)

	tagRegex, err := common.BuildRegexFilter(tagFilter, regexpMatchTimeoutSeconds)
	if err != nil {
		return -1, err
	}
	lastTag := ""
	skippedTagsCount := 0
	deletedTagsCount := 0
	// In order to only have a limited amount of http requests, a purger is used that will start goroutines to delete tags.
	purger := worker.NewPurger(poolSize, acrClient, loginURL, repoName)
	// GetTagsToDelete will return an empty lastTag when there are no more tags.
	for {
		tagsToDelete, newLastTag, newSkippedTagsCount, err := getTagsToDelete(ctx, acrClient, repoName, tagRegex, timeToCompare, lastTag, keep, skippedTagsCount)
		if err != nil {
			return -1, err
		}
		lastTag = newLastTag
		skippedTagsCount = newSkippedTagsCount
		if tagsToDelete != nil {
			count, purgeErr := purger.PurgeTags(ctx, tagsToDelete)
			if purgeErr != nil {
				return -1, purgeErr
			}
			deletedTagsCount += count
		}
		if len(lastTag) == 0 {
			break
		}
	}

	return deletedTagsCount, nil
}

// parseDuration analog to time.ParseDuration() but with days added.
func parseDuration(ago string) (time.Duration, error) {
	var days int
	var durationString string
	// The supported format is %d%s where the string is a valid go duration string.
	if strings.Contains(ago, "d") {
		if _, err := fmt.Sscanf(ago, "%dd%s", &days, &durationString); err != nil {
			fmt.Sscanf(ago, "%dd", &days)
			durationString = ""
		}
	} else {
		days = 0
		if _, err := fmt.Sscanf(ago, "%s", &durationString); err != nil {
			return time.Duration(0), err
		}
	}
	// The number of days gets converted to hours.
	duration := time.Duration(days) * 24 * time.Hour
	if len(durationString) > 0 {
		agoDuration, err := time.ParseDuration(durationString)
		if err != nil {
			return time.Duration(0), err
		}
		duration = duration + agoDuration
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
	skippedTagsCount int) (*[]acr.TagAttributesBase, string, int, error) {

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
			// as a tag to delete.
			if lastUpdateTime.Before(timeToCompare) && *(*tag.ChangeableAttributes).DeleteEnabled && *(*tag.ChangeableAttributes).WriteEnabled {
				tagsEligibleForDeletion = append(tagsEligibleForDeletion, tag)
			}
		}

		newLastTag = common.GetLastTagFromResponse(resultTags)
		// No more tags to keep
		if keep == 0 || skippedTagsCount == keep {
			return &tagsEligibleForDeletion, newLastTag, skippedTagsCount, nil
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
		return &tagsToDelete, newLastTag, skippedTagsCount, nil
	}
	// In case there are no more tags return empty string as lastTag so that the purgeTags function stops
	return nil, "", skippedTagsCount, nil
}

// purgeDanglingManifests deletes all manifests that do not have any tags associated with them.
// except the ones that are referenced by a multiarch manifest or that have subject.
func purgeDanglingManifests(ctx context.Context, acrClient api.AcrCLIClientInterface, poolSize int, loginURL string, repoName string) (int, error) {
	fmt.Printf("Deleting manifests for repository: %s\n", repoName)
	// Contrary to getTagsToDelete, getManifestsToDelete gets all the Manifests at once, this was done because if there is a manifest that has no
	// tag but is referenced by a multiarch manifest that has tags then it should not be deleted. Or if a manifest has no tag, but it has subject,
	// then it should not be deleted.
	manifestsToDelete, err := common.GetUntaggedManifests(ctx, acrClient, loginURL, repoName, false, true)
	if err != nil {
		return -1, err
	}
	// In order to only have a limited amount of http requests, a purger is used that will start goroutines to delete manifests.
	purger := worker.NewPurger(poolSize, acrClient, loginURL, repoName)
	deletedManifestsCount, purgeErr := purger.PurgeManifests(ctx, manifestsToDelete)
	if purgeErr != nil {
		return -1, purgeErr
	}
	return deletedManifestsCount, nil
}

// dryRunPurge outputs everything that would be deleted if the purge command was executed
func dryRunPurge(ctx context.Context, acrClient api.AcrCLIClientInterface, loginURL string, repoName string, ago string, filter string, untagged bool, keep int, regexMatchTimeout uint64) (int, int, error) {
	deletedTagsCount := 0
	deletedManifestsCount := 0
	// In order to keep track if a manifest would get deleted a map is defined that as a key has the manifest
	// digest and as the value the number of tags (referencing said manifests) that were deleted.
	deletedTags := map[string]int{}
	fmt.Printf("Tags for this repository would be deleted: %s\n", repoName)
	agoDuration, err := parseDuration(ago)
	if err != nil {
		return -1, -1, err
	}
	timeToCompare := time.Now().UTC()
	timeToCompare = timeToCompare.Add(agoDuration)
	regex, err := common.BuildRegexFilter(filter, regexMatchTimeout)
	if err != nil {
		return -1, -1, err
	}

	lastTag := ""
	skippedTagsCount := 0
	// The loop to get the deleted tags follows the same logic as the one in the purgeTags function
	for {
		tagsToDelete, newLastTag, newSkippedTagsCount, err := getTagsToDelete(ctx, acrClient, repoName, regex, timeToCompare, lastTag, keep, skippedTagsCount)
		if err != nil {
			return -1, -1, err
		}
		lastTag = newLastTag
		skippedTagsCount = newSkippedTagsCount
		if tagsToDelete != nil {
			for _, tag := range *tagsToDelete {
				// For every tag that would be deleted first check if it exists in the map, if it doesn't add a new key
				// with value 1 and if it does just add 1 to the existent value.
				deletedTags[*tag.Digest]++
				fmt.Printf("%s/%s:%s\n", loginURL, repoName, *tag.Name)
				deletedTagsCount++
			}
		}
		if len(lastTag) == 0 {
			break
		}
	}
	if untagged {
		fmt.Printf("Manifests for this repository would be deleted: %s\n", repoName)
		// The tagsCount contains a map that for every digest contains how many tags are referencing it.
		tagsCount, err := countTagsByManifest(ctx, acrClient, repoName)
		if err != nil {
			return -1, -1, err
		}
		lastManifestDigest := ""
		resultManifests, err := acrClient.GetAcrManifests(ctx, repoName, "", lastManifestDigest)
		if err != nil {
			if resultManifests != nil && resultManifests.Response.Response != nil && resultManifests.StatusCode == http.StatusNotFound {
				fmt.Printf("%s repository not found\n", repoName)
				return 0, 0, nil
			}
			return -1, -1, err
		}
		// This set will do the check: if a key is present, then the corresponding manifest
		// should not be deleted because it is referenced by a multiarch manifest
		// or otherwise references a subject
		doNotDelete := set.New[string]()
		candidatesToDelete := []acr.ManifestAttributesBase{}
		for resultManifests != nil && resultManifests.ManifestsAttributes != nil {
			manifests := *resultManifests.ManifestsAttributes
			for _, manifest := range manifests {
				if tagsCount[*manifest.Digest] != deletedTags[*manifest.Digest] {
					// If the number of the tags associated with the manifest is  not equal to
					// the number of tags to be deleted, the said manifest will still have
					// tags remaining and we will consider if it has any dependant manifests
					err = common.AddDependentManifestsToIgnoreList(ctx, manifest, doNotDelete, acrClient, repoName)
					if err != nil {
						return -1, -1, err
					}
				} else {
					// Otherwise the manifest have no tag left after purgeTags.
					// we will need to find if the manifest has subject. If so,
					// we will mark the manifest to not be deleted.
					candidatesToDelete, err = common.UpdateForManifestWithoutSubjectToDelete(ctx, manifest, doNotDelete, candidatesToDelete, acrClient, repoName)
					if err != nil {
						return -1, -1, err
					}
				}
			}
			// Get the last manifest digest from the last manifest from manifests.
			lastManifestDigest = *manifests[len(manifests)-1].Digest
			// Use this new digest to find next batch of manifests.
			resultManifests, err = acrClient.GetAcrManifests(ctx, repoName, "", lastManifestDigest)
			if err != nil {
				return -1, -1, err
			}
		}
		// Just print manifests that would be deleted.
		for i := 0; i < len(candidatesToDelete); i++ {
			if !doNotDelete.Contains(*candidatesToDelete[i].Digest) {
				fmt.Printf("%s/%s@%s\n", loginURL, repoName, *candidatesToDelete[i].Digest)
				deletedManifestsCount++
			}
		}
	}

	return deletedTagsCount, deletedManifestsCount, nil
}

// countTagsByManifest returns a map that for a given manifest digest contains the number of tags associated to it.
func countTagsByManifest(ctx context.Context, acrClient api.AcrCLIClientInterface, repoName string) (map[string]int, error) {
	tagsCount := map[string]int{}
	lastTag := ""
	resultTags, err := acrClient.GetAcrTags(ctx, repoName, "", lastTag)
	if err != nil {
		if resultTags != nil && resultTags.Response.Response != nil && resultTags.StatusCode == http.StatusNotFound {
			//Repository not found, will be handled in the GetAcrManifests call
			return nil, nil
		}
		return nil, err
	}
	for resultTags != nil && resultTags.TagsAttributes != nil {
		tags := *resultTags.TagsAttributes
		for _, tag := range tags {
			// if a digest already exists in the map then add 1 to the number of tags it has.
			tagsCount[*tag.Digest]++
		}

		lastTag = *tags[len(tags)-1].Name
		// Keep on iterating until the resultTags or resultTags.TagsAttributes is nil
		resultTags, err = acrClient.GetAcrTags(ctx, repoName, "", lastTag)
		if err != nil {
			return nil, err
		}
	}
	return tagsCount, nil
}
