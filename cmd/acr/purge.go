// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package main

import (
	"context"
	"fmt"
	"net/http"
	"runtime"
	"sort"
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
	purgeExampleMessage    = `  TAG DELETION EXAMPLES:
  - Delete all tags that are older than 1 day in the hello-world repository
    	acr purge -r example --filter "hello-world:.*" --ago 1d

  - Delete all tags that are older than 7 days in all repositories
	    acr purge -r example --filter ".*:.*" --ago 7d 

  - Delete tags older than 7 days that begin with "hello", keeping the latest 2
    	acr purge -r example --filter "hello-world:^hello.*" --ago 7d --keep 2

  - Delete tags containing "test" that are older than 5 days, then delete any dangling (untagged) manifests older than 5 days
	acr purge -r example --filter "hello-world:\w*test\w*" --ago 5d --untagged 

  DANGLING MANIFEST CLEANUP EXAMPLES (--untagged-only is the primary way to clean up dangling manifests):
  - Clean up ALL dangling manifests in all repositories
	acr purge -r example --untagged-only

  - Clean up dangling manifests only in the hello-world repository
	acr purge -r example --filter "hello-world:.*" --untagged-only

  - Clean up dangling manifests older than 3 days, keeping the 5 most recent
	acr purge -r example --untagged-only --ago 3d --keep 5

  ADVANCED OPTIONS:
  - Use custom authentication config
	acr purge -r example --filter "hello-world:.*" --ago 1d --config C://Users/docker/config.json

  - Run with custom concurrency (4 parallel tasks)
	acr purge -r example --filter "hello-world:.*" --ago 1d --concurrency 4

  - Use custom page size for repository queries
	acr purge -r example --filter ".*:.*" --ago 7d --repository-page-size 50

  - Include locked manifests/tags in deletion
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
	untaggedOnly  bool
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
			// Validate flag combinations before authentication
			// untagged-only mode: filter and ago are optional (skip validation)
			// untagged mode: requires filter and ago (it's a cleanup step after tag deletion)
			// standard mode: requires filter and ago for tag deletion
			if purgeParams.untagged {
				if len(purgeParams.filters) == 0 {
					return fmt.Errorf("--filter is required when using --untagged")
				}
				if purgeParams.ago == "" {
					return fmt.Errorf("--ago is required when using --untagged")
				}
			} else if !purgeParams.untaggedOnly {
				if len(purgeParams.filters) == 0 {
					return fmt.Errorf("--filter is required when not using --untagged-only")
				}
				if purgeParams.ago == "" {
					return fmt.Errorf("--ago is required when not using --untagged-only")
				}
			}

			// Parse and validate duration early (before authentication)
			var agoDuration time.Duration
			if purgeParams.ago == "" {
				// Use 0 duration so timeToCompare equals now, meaning all past manifests are eligible
				agoDuration = 0
			} else {
				var err error
				agoDuration, err = parseDuration(purgeParams.ago)
				if err != nil {
					return err
				}
			}

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
			var tagFilters map[string]string
			if purgeParams.untaggedOnly && len(purgeParams.filters) == 0 {
				// If untagged-only without filters, get all repositories
				allRepoNames, err := repository.GetAllRepositoryNames(ctx, acrClient.AutorestClient, purgeParams.repoPageSize)
				if err != nil {
					return err
				}
				tagFilters = make(map[string]string)
				for _, repoName := range allRepoNames {
					tagFilters[repoName] = "" // empty filter - won't be used in untagged-only mode
				}
			} else if len(purgeParams.filters) > 0 {
				tagFilters, err = repository.CollectTagFilters(ctx, purgeParams.filters, acrClient.AutorestClient, purgeParams.filterTimeout, purgeParams.repoPageSize)
				if err != nil {
					return err
				}
			} else {
				tagFilters = make(map[string]string)
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

			// Combine flags for clarity - these are mutually exclusive
			supportUntaggedCleanup := purgeParams.untagged || purgeParams.untaggedOnly

			deletedTagsCount, deletedManifestsCount, err := purge(ctx, acrClient, loginURL, repoParallelism, agoDuration, purgeParams.keep, purgeParams.filterTimeout, supportUntaggedCleanup, purgeParams.untaggedOnly, tagFilters, purgeParams.dryRun, purgeParams.includeLocked)

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

	cmd.Flags().BoolVar(&purgeParams.untagged, "untagged", false, "In addition to deleting tags (based on --filter and --ago), also delete untagged manifests that were left behind after tag deletion. This is typically used as a cleanup step after deleting tags. Note: This requires --filter and --ago to be specified")
	cmd.Flags().BoolVar(&purgeParams.untaggedOnly, "untagged-only", false, "Clean up dangling manifests: Delete ONLY untagged manifests (manifests without any tags), without deleting any tags first. This is the primary way to clean up dangling manifests in your registry. Optional: Use --ago to delete only old untagged manifests, --keep to preserve recent ones, and --filter to target specific repositories. Note: Only the repository portion of --filter is used; the tag regex portion is ignored")
	cmd.Flags().BoolVar(&purgeParams.dryRun, "dry-run", false, "If the dry-run flag is set no manifest or tag will be deleted, the output would be the same as if they were deleted")
	cmd.Flags().BoolVar(&purgeParams.includeLocked, "include-locked", false, "If the include-locked flag is set, locked manifests and tags (where deleteEnabled or writeEnabled is false) will be unlocked before deletion")
	cmd.Flags().StringVar(&purgeParams.ago, "ago", "", "Delete tags or untagged manifests that were last updated before this duration. Format: [number]d[string] where the first number represents days and the string is in Go duration format (e.g. 2d3h6m selects images older than 2 days, 3 hours and 6 minutes). Required when deleting tags, optional with --untagged-only. Maximum duration is capped at 150 years to prevent overflow")
	cmd.Flags().IntVar(&purgeParams.keep, "keep", 0, "Number of latest to-be-deleted items to keep. For tag deletion: keep the x most recent tags that would otherwise be deleted. For --untagged-only: keep the x most recent untagged manifests")
	cmd.Flags().StringArrayVarP(&purgeParams.filters, "filter", "f", nil, "Specify the repository and a regular expression filter for the tag name, if a tag matches the filter and is older than the duration specified in ago it will be deleted. Note: If backtracking is used in the regexp it's possible for the expression to run into an infinite loop. The default timeout is set to 1 minute for evaluation of any filter expression. Use the '--filter-timeout-seconds' option to set a different value.")
	cmd.Flags().StringArrayVarP(&purgeParams.configs, "config", "c", nil, "Authentication config paths (e.g. C://Users/docker/config.json)")
	cmd.Flags().Int64Var(&purgeParams.filterTimeout, "filter-timeout-seconds", defaultRegexpMatchTimeoutSeconds, "This limits the evaluation of the regex filter, and will return a timeout error if this duration is exceeded during a single evaluation. If written incorrectly a regexp filter with backtracking can result in an infinite loop.")
	cmd.Flags().IntVar(&purgeParams.concurrency, "concurrency", defaultPoolSize, concurrencyDescription)
	cmd.Flags().Int32Var(&purgeParams.repoPageSize, "repository-page-size", defaultRepoPageSize, repoPageSizeDescription)
	cmd.Flags().BoolP("help", "h", false, "Print usage")
	// Make filter and ago conditionally required based on untagged-only flag
	cmd.MarkFlagsOneRequired("filter", "untagged-only")
	cmd.MarkFlagsMutuallyExclusive("untagged", "untagged-only")
	return cmd
}

func purge(ctx context.Context,
	acrClient api.AcrCLIClientInterface,
	loginURL string,
	repoParallelism int,
	agoDuration time.Duration,
	keep int,
	filterTimeout int64,
	removeUntaggedManifests bool,
	untaggedOnly bool,
	tagFilters map[string]string,
	dryRun bool,
	includeLocked bool) (deletedTagsCount int, deletedManifestsCount int, err error) {

	// In order to print a summary of the deleted tags/manifests the counters get updated everytime a repo is purged.
	for repoName, tagRegex := range tagFilters {
		var singleDeletedTagsCount int
		var manifestToTagsCountMap map[string]int

		// Handle tag deletion based on mode
		if untaggedOnly {
			// Initialize empty map for untagged-only mode (no tag deletion)
			manifestToTagsCountMap = make(map[string]int)
		} else {
			// Standard mode: delete matching tags first
			singleDeletedTagsCount, manifestToTagsCountMap, err = purgeTags(ctx, acrClient, repoParallelism, loginURL, repoName, agoDuration, tagRegex, keep, filterTimeout, dryRun, includeLocked)
			if err != nil {
				return deletedTagsCount, deletedManifestsCount, fmt.Errorf("failed to purge tags: %w", err)
			}
		}

		singleDeletedManifestsCount := 0
		// If the untagged flag is set or untagged-only mode is enabled, delete manifests
		if removeUntaggedManifests {
			singleDeletedManifestsCount, err = purgeDanglingManifests(ctx, acrClient, repoParallelism, loginURL, repoName, agoDuration, keep, manifestToTagsCountMap, dryRun, includeLocked)
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

// sortManifestsByTime sorts manifests by LastUpdateTime (newest first) with consistent
// handling of nil or unparseable timestamps and a digest-based tie-breaker for determinism.
func sortManifestsByTime(manifests []acr.ManifestAttributesBase) {
	sort.Slice(manifests, func(i, j int) bool {
		mi := manifests[i]
		mj := manifests[j]

		// Extract timestamp strings; nil means invalid
		var si, sj string
		if mi.LastUpdateTime != nil {
			si = *mi.LastUpdateTime
		}
		if mj.LastUpdateTime != nil {
			sj = *mj.LastUpdateTime
		}

		// Parse timestamps, trying RFC3339Nano first then RFC3339
		ti, errI := time.Parse(time.RFC3339Nano, si)
		if errI != nil && si != "" {
			if t, err := time.Parse(time.RFC3339, si); err == nil {
				ti, errI = t, nil
			}
		}
		tj, errJ := time.Parse(time.RFC3339Nano, sj)
		if errJ != nil && sj != "" {
			if t, err := time.Parse(time.RFC3339, sj); err == nil {
				tj, errJ = t, nil
			}
		}

		// Define validity flags
		vi := (errI == nil)
		vj := (errJ == nil)

		// Ordering rules (total order):
		// 1) Valid timestamps come before invalid (invalid considered oldest, so deleted first)
		if vi != vj {
			return vi // true if i valid and j invalid (i should be kept, j deleted)
		}

		// 2) Both valid: newest first
		if vi && vj {
			if ti.Equal(tj) {
				// 3) Tie-breaker for determinism using digest
				if mi.Digest != nil && mj.Digest != nil {
					return *mi.Digest < *mj.Digest
				}
				return false
			}
			return ti.After(tj) // newest first
		}

		// 4) Both invalid: tie-breaker for determinism using digest
		if mi.Digest != nil && mj.Digest != nil {
			return *mi.Digest < *mj.Digest
		}
		return false
	})
}

// purgeDanglingManifests deletes all manifests that do not have any tags associated with them.
// except the ones that are referenced by a multiarch manifest or that have subject.
// If keep is provided, the specified number of most recent manifests will be kept.
func purgeDanglingManifests(ctx context.Context, acrClient api.AcrCLIClientInterface, repoParallelism int, loginURL string, repoName string, agoDuration time.Duration, keep int, manifestToTagsCountMap map[string]int, dryRun bool, includeLocked bool) (int, error) {
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

	// Apply keep logic if keep parameter is provided
	if keep > 0 && len(manifestsToDelete) > keep {
		// Sort manifests by LastUpdateTime (newest first) using sortManifestsByTime
		sortManifestsByTime(manifestsToDelete)
		// Keep only manifests after the 'keep' count
		manifestsToDelete = manifestsToDelete[keep:]
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
