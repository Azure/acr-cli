// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Azure/acr-cli/acr"
	"github.com/Azure/acr-cli/cmd/repository"
	"github.com/Azure/acr-cli/internal/api"
	"github.com/Azure/acr-cli/internal/worker"
	"github.com/Azure/go-autorest/autorest"
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

  - Protect the newest 10 matching tags; delete other matching tags if older than 3 days or outside the newest 50
	acr purge -r example --filter "hello-world:.*" --ago 3d --min-tags 10 --max-tags 50

  - Retain only the newest 10 matching tags.
	acr purge -r example --filter "hello-world:.*" --max-tags 10

  - Delete tags containing "test" that are older than 5 days, then delete any dangling (untagged) manifests older than 5 days
	acr purge -r example --filter "hello-world:\w*test\w*" --ago 5d --untagged 

  DANGLING MANIFEST CLEANUP EXAMPLES (--untagged-only is the primary way to clean up dangling manifests):
  - Clean up ALL dangling manifests in all repositories
	acr purge -r example --untagged-only

  - Clean up dangling manifests only in the hello-world repository
	acr purge -r example --filter "hello-world:.*" --untagged-only

  - Clean up dangling manifests older than 3 days, keeping the 5 most recent
	acr purge -r example --untagged-only --ago 3d --keep 5

  - Protect the newest 20 dangling manifests; delete other dangling manifests if older than 3 days or outside the newest 100
	acr purge -r example --untagged-only --ago 3d --min-untagged-manifests 20 --max-untagged-manifests 100

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
	ago                  string
	keep                 int
	minTags              int
	maxTags              int
	minUntaggedManifests int
	maxUntaggedManifests int
	filters              []string
	filterTimeout        int64
	untagged             bool
	untaggedOnly         bool
	dryRun               bool
	includeLocked        bool
	concurrency          int
	repoPageSize         int32
	verbose              bool
}

// newPurgeCmd defines the purge command.
func newPurgeCmd(rootParams *rootParameters) *cobra.Command {
	purgeParams := purgeParameters{rootParameters: rootParams}
	cmd := &cobra.Command{
		Use:     "purge",
		Short:   "Delete images from a registry.",
		Long:    newPurgeCmdLongMessage,
		Example: purgeExampleMessage,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Validate flag combinations before authentication
			if cmd.Flags().Changed("min-tags") && purgeParams.minTags < 0 {
				return fmt.Errorf("--min-tags must be nonnegative")
			}
			if cmd.Flags().Changed("max-tags") && purgeParams.maxTags < 0 {
				return fmt.Errorf("--max-tags must be nonnegative")
			}
			if cmd.Flags().Changed("min-untagged-manifests") && purgeParams.minUntaggedManifests < 0 {
				return fmt.Errorf("--min-untagged-manifests must be nonnegative")
			}
			if cmd.Flags().Changed("max-untagged-manifests") && purgeParams.maxUntaggedManifests < 0 {
				return fmt.Errorf("--max-untagged-manifests must be nonnegative")
			}
			if purgeParams.minTags >= 0 {
				if purgeParams.ago == "" {
					return fmt.Errorf("--min-tags requires --ago")
				}
				if purgeParams.untaggedOnly {
					return fmt.Errorf("--min-tags cannot be combined with --untagged-only")
				}
			}
			if purgeParams.maxTags >= 0 && purgeParams.untaggedOnly {
				return fmt.Errorf("--max-tags cannot be combined with --untagged-only")
			}
			if purgeParams.minUntaggedManifests >= 0 {
				if purgeParams.ago == "" {
					return fmt.Errorf("--min-untagged-manifests requires --ago")
				}
				if !purgeParams.untagged && !purgeParams.untaggedOnly {
					return fmt.Errorf("--min-untagged-manifests requires --untagged or --untagged-only")
				}
			}
			if purgeParams.maxUntaggedManifests >= 0 && !purgeParams.untagged && !purgeParams.untaggedOnly {
				return fmt.Errorf("--max-untagged-manifests requires --untagged or --untagged-only")
			}
			if purgeParams.minTags >= 0 && purgeParams.maxTags >= 0 && purgeParams.minTags > purgeParams.maxTags {
				return fmt.Errorf("--min-tags must not exceed --max-tags")
			}
			if purgeParams.minUntaggedManifests >= 0 && purgeParams.maxUntaggedManifests >= 0 && purgeParams.minUntaggedManifests > purgeParams.maxUntaggedManifests {
				return fmt.Errorf("--min-untagged-manifests must not exceed --max-untagged-manifests")
			}
			if !purgeParams.untaggedOnly {
				if len(purgeParams.filters) == 0 {
					return fmt.Errorf("--filter is required when not using --untagged-only")
				}
				if purgeParams.ago == "" && purgeParams.maxTags < 0 {
					return fmt.Errorf("--ago or --max-tags is required when not using --untagged-only")
				}
			}

			// Parse and validate duration early (before authentication)
			var agoDuration *time.Duration
			if purgeParams.ago != "" {
				duration, err := parseDuration(purgeParams.ago)
				if err != nil {
					return err
				}
				agoDuration = &duration
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

			deletedTagsCount, deletedManifestsCount, err := purge(ctx, acrClient, loginURL, repoParallelism, agoDuration, purgeParams.keep, purgeParams.minTags, purgeParams.maxTags, purgeParams.minUntaggedManifests, purgeParams.maxUntaggedManifests, purgeParams.filterTimeout, supportUntaggedCleanup, purgeParams.untaggedOnly, tagFilters, purgeParams.dryRun, purgeParams.includeLocked, purgeParams.verbose)

			if err != nil && !strings.Contains(err.Error(), "insufficient permissions") {
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

	cmd.Flags().BoolVar(&purgeParams.untagged, "untagged", false, "After deleting matching tags, also delete eligible dangling manifests. Requires --filter and either --ago or --max-tags. Without --ago or manifest count limits, all dangling manifests are eligible")
	cmd.Flags().BoolVar(&purgeParams.untaggedOnly, "untagged-only", false, "Clean up dangling manifests: Delete ONLY untagged manifests (manifests without any tags), without deleting any tags first. This is the primary way to clean up dangling manifests in your registry. Optional: Use --ago to delete only old untagged manifests, --keep to preserve recent ones, and --filter to target specific repositories. Note: Only the repository portion of --filter is used; the tag regex portion is ignored")
	cmd.Flags().BoolVar(&purgeParams.dryRun, "dry-run", false, "If the dry-run flag is set no manifest or tag will be deleted, the output would be the same as if they were deleted")
	cmd.Flags().BoolVar(&purgeParams.includeLocked, "include-locked", false, "If the include-locked flag is set, locked manifests and tags (where deleteEnabled or writeEnabled is false) will be unlocked before deletion")
	cmd.Flags().StringVar(&purgeParams.ago, "ago", "", "Delete tags or untagged manifests that were last updated before this duration. Format: [number]d[string] where the first number represents days and the string is in Go duration format (e.g. 2d3h6m selects images older than 2 days, 3 hours and 6 minutes). Required with minimum limits and when deleting tags without --max-tags. Maximum duration is capped at 150 years to prevent overflow")
	cmd.Flags().IntVar(&purgeParams.keep, "keep", 0, "Number of latest to-be-deleted items to keep. For tag deletion: keep the x most recent tags that would otherwise be deleted. For --untagged-only: keep the x most recent eligible untagged manifests. Cannot be combined with any min/max retention flag")
	cmd.Flags().IntVar(&purgeParams.minTags, "min-tags", -1, "Retention floor. Always keep the newest N matching tags. Requires --ago. N >= 0, N <= max-tags if specified; cannot be combined with --keep.")
	cmd.Flags().IntVar(&purgeParams.maxTags, "max-tags", -1, "Retention cap. Never keep more than the newest N matching tags. N >= 0, N >= min-tags if specified; cannot be combined with --keep.")
	cmd.Flags().IntVar(&purgeParams.minUntaggedManifests, "min-untagged-manifests", -1, "Retention floor. Always keep the newest N dangling manifests. Requires --ago. N >= 0, N <= max-untagged-manifests if specified; cannot be combined with --keep.")
	cmd.Flags().IntVar(&purgeParams.maxUntaggedManifests, "max-untagged-manifests", -1, "Retention cap. Never keep more than the newest N dangling manifests. N >= 0, N >= min-untagged-manifests if specified; cannot be combined with --keep.")
	cmd.Flags().StringArrayVarP(&purgeParams.filters, "filter", "f", nil, "Specify the repository and a regular expression filter for the tag name. Matching tags are selected by age and optional count policies; filters for the same repository are combined. Note: If backtracking is used in the regexp it's possible for the expression to run into an infinite loop. The default timeout is set to 1 minute for evaluation of any filter expression. Use the '--filter-timeout-seconds' option to set a different value.")
	cmd.Flags().StringArrayVarP(&purgeParams.configs, "config", "c", nil, "Authentication config paths (e.g. C://Users/docker/config.json)")
	cmd.Flags().Int64Var(&purgeParams.filterTimeout, "filter-timeout-seconds", defaultRegexpMatchTimeoutSeconds, "This limits the evaluation of the regex filter, and will return a timeout error if this duration is exceeded during a single evaluation. If written incorrectly a regexp filter with backtracking can result in an infinite loop.")
	cmd.Flags().IntVar(&purgeParams.concurrency, "concurrency", defaultPoolSize, concurrencyDescription)
	cmd.Flags().Int32Var(&purgeParams.repoPageSize, "repository-page-size", defaultRepoPageSize, repoPageSizeDescription)
	cmd.Flags().BoolVar(&purgeParams.verbose, "verbose", false, "Enable verbose output including detailed repository names during ABAC token operations")
	cmd.Flags().BoolP("help", "h", false, "Print usage")
	// Make filter and ago conditionally required based on untagged-only flag
	cmd.MarkFlagsOneRequired("filter", "untagged-only")
	cmd.MarkFlagsMutuallyExclusive("untagged", "untagged-only")
	// Make keep and min/max flags mutually exclusive
	cmd.MarkFlagsMutuallyExclusive("keep", "min-tags")
	cmd.MarkFlagsMutuallyExclusive("keep", "max-tags")
	cmd.MarkFlagsMutuallyExclusive("keep", "min-untagged-manifests")
	cmd.MarkFlagsMutuallyExclusive("keep", "max-untagged-manifests")
	return cmd
}

func purge(ctx context.Context,
	acrClient api.AcrCLIClientInterface,
	loginURL string,
	repoParallelism int,
	agoDuration *time.Duration,
	keep int,
	minTags int,
	maxTags int,
	minManifests int,
	maxManifests int,
	filterTimeout int64,
	removeUntaggedManifests bool,
	untaggedOnly bool,
	tagFilters map[string]string,
	dryRun bool,
	includeLocked bool,
	verbose bool) (deletedTagsCount int, deletedManifestsCount int, err error) {

	// Load ABAC batch size from environment variable
	abacBatchSize := 10 // default
	if envVal, exists := os.LookupEnv("ABAC_BATCH_SIZE"); exists {
		if parsed, err := strconv.Atoi(envVal); err == nil && parsed > 0 {
			abacBatchSize = parsed
		}
	}

	// Collect all repository names into a sorted slice for deterministic batching and output.
	repos := make([]string, 0, len(tagFilters))
	for repoName := range tagFilters {
		repos = append(repos, repoName)
	}
	sort.Strings(repos)

	// Track which repositories have been successfully processed for error reporting.
	var completedRepos []string

	// Process repositories in batches of abacBatchSize.
	// For ABAC-enabled registries, we set the current repositories for the batch so that
	// token refresh happens dynamically when needed (on API calls that detect token expiration).
	// For non-ABAC registries, the batching loop is harmless (no special token handling needed).
	for i := 0; i < len(repos); i += abacBatchSize {
		end := i + abacBatchSize
		if end > len(repos) {
			end = len(repos)
		}
		batch := repos[i:end]

		// For ABAC registries, refresh the token with scopes for this batch of repositories.
		// ABAC registries don't support wildcard repository scopes, so we must explicitly
		// request access for each repository before operating on it.
		if acrClient.IsAbac() {
			if err := acrClient.RefreshTokenForAbac(ctx, batch); err != nil {
				return deletedTagsCount, deletedManifestsCount, fmt.Errorf("failed to refresh ABAC token for batch: %w", err)
			}
			if verbose {
				fmt.Printf("ABAC: Setting token scope for %d repositories: %v\n", len(batch), batch)
			} else {
				fmt.Printf("ABAC: Setting token scope for %d repositories\n", len(batch))
			}
		}

		// Process all repositories in this batch
		for _, repoName := range batch {
			tagRegex := tagFilters[repoName]
			var singleDeletedTagsCount int
			var manifestToTagsCountMap map[string]int

			// Handle tag deletion based on mode
			if untaggedOnly {
				// Initialize empty map for untagged-only mode (no tag deletion)
				manifestToTagsCountMap = make(map[string]int)
			} else {
				// Standard mode: delete matching tags first
				singleDeletedTagsCount, manifestToTagsCountMap, err = purgeTags(ctx, acrClient, repoParallelism, loginURL, repoName, agoDuration, tagRegex, keep, minTags, maxTags, filterTimeout, dryRun, includeLocked)
				if err != nil {
					if isUnauthorizedError(err) {
						remainingRepos := repos[i+indexOf(batch, repoName):]
						return deletedTagsCount, deletedManifestsCount,
							formatPermissionError(repoName, "purge tags", completedRepos, remainingRepos)
					}
					return deletedTagsCount, deletedManifestsCount, fmt.Errorf("failed to purge tags: %w", err)
				}
			}

			singleDeletedManifestsCount := 0
			// If the untagged flag is set or untagged-only mode is enabled, delete manifests
			if removeUntaggedManifests {
				singleDeletedManifestsCount, err = purgeDanglingManifests(ctx, acrClient, repoParallelism, loginURL, repoName, agoDuration, keep, minManifests, maxManifests, manifestToTagsCountMap, dryRun, includeLocked)
				if err != nil {
					if isUnauthorizedError(err) {
						remainingRepos := repos[i+indexOf(batch, repoName):]
						return deletedTagsCount, deletedManifestsCount,
							formatPermissionError(repoName, "purge manifests", completedRepos, remainingRepos)
					}
					return deletedTagsCount, deletedManifestsCount, fmt.Errorf("failed to purge manifests: %w", err)
				}
			}
			// After every repository is purged the counters are updated.
			deletedTagsCount += singleDeletedTagsCount
			deletedManifestsCount += singleDeletedManifestsCount
			completedRepos = append(completedRepos, repoName)
		}
	}

	return deletedTagsCount, deletedManifestsCount, nil

}

// purgeTags deletes all tags that are older than the agoDuration value, that match the tagFilter string and are subject to the keep, minTags, and maxTags parameters.
func purgeTags(ctx context.Context, acrClient api.AcrCLIClientInterface, repoParallelism int, loginURL string, repoName string, agoDuration *time.Duration, tagFilter string, keep int, minTags int, maxTags int, regexpMatchTimeoutSeconds int64, dryRun bool, includeLocked bool) (int, map[string]int, error) {
	if dryRun {
		fmt.Printf("Would delete tags for repository: %s\n", repoName)
	} else {
		fmt.Printf("Deleting tags for repository: %s\n", repoName)
	}
	manifestToTagsCountMap := make(map[string]int) // This map is used to keep track of how many tags would have been deleted per manifest.
	timeToCompare := time.Now().UTC()
	if agoDuration != nil {
		// Since the parseDuration function returns a negative duration, it is added to the current duration in order to be able to easily compare
		// with the LastUpdatedTime attribute a tag has.
		timeToCompare = timeToCompare.Add(*agoDuration)
	} else if maxTags >= 0 {
		// A zero cutoff disables age pruning only for count policies.
		timeToCompare = time.Time{}
	}

	tagRegex, err := repository.BuildRegexFilter(tagFilter, regexpMatchTimeoutSeconds)
	if err != nil {
		return -1, manifestToTagsCountMap, fmt.Errorf("failed to build Regex %s with error: %w", tagRegex, err)
	}

	lastTag := ""
	skippedTagsCount := 0
	processedMatchingTagsCount := 0
	deletedTagsCount := 0
	// In order to only have a limited amount of http requests, a purger is used that will start goroutines to delete tags.
	purger := worker.NewPurger(repoParallelism, acrClient, loginURL, repoName, includeLocked)

	for {
		tagsToDelete, newLastTag, newSkippedTagsCount, newProcessedMatchingTagsCount, err := getTagsToDelete(ctx, acrClient, repoName, tagRegex, timeToCompare, lastTag, keep, minTags, maxTags, skippedTagsCount, processedMatchingTagsCount, includeLocked)
		if err != nil {
			return -1, manifestToTagsCountMap, err
		}
		lastTag = newLastTag
		skippedTagsCount = newSkippedTagsCount
		processedMatchingTagsCount = newProcessedMatchingTagsCount
		if len(tagsToDelete) > 0 {
			tagAttributes := make([]acr.TagAttributesBase, 0, len(tagsToDelete))
			for _, tag := range tagsToDelete {
				manifestToTagsCountMap[*tag.Digest]++
				if dryRun {
					if reason := tag.Reason.String(); reason != "" {
						fmt.Printf("Would delete: %s/%s:%s (reason: %s)\n", loginURL, repoName, *tag.Name, reason)
					} else {
						fmt.Printf("Would delete: %s/%s:%s\n", loginURL, repoName, *tag.Name)
					}
				}
				tagAttributes = append(tagAttributes, tag.TagAttributesBase)
			}

			if dryRun {
				deletedTagsCount += len(tagsToDelete)
				if len(lastTag) == 0 {
					break
				}
				continue // If dryRun is set to true then no tags will be deleted, but the count is updated.
			}

			count, purgeErr := purger.PurgeTags(ctx, tagAttributes)
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

// getTagsToDelete returns one page's eligible tags, continuation, legacy keep count, and matching rank.
// Count policies carry rank across pages in the API's timedesc order.
// A zero timeToCompare disables their age clause; locked tags count toward rank before exclusion.
func getTagsToDelete(ctx context.Context,
	acrClient api.AcrCLIClientInterface,
	repoName string,
	filter *regexp2.Regexp,
	timeToCompare time.Time,
	lastTag string,
	keep int,
	minTags int,
	maxTags int,
	skippedTagsCount int,
	processedMatchingTagsCount int,
	includeLocked bool) ([]repository.TagToDelete, string, int, int, error) {

	var matches bool
	var lastUpdateTime time.Time
	var deleteCutoff *time.Time
	if !timeToCompare.IsZero() {
		deleteCutoff = &timeToCompare
	}
	resultTags, err := acrClient.GetAcrTags(ctx, repoName, "timedesc", lastTag)
	if err != nil {
		if resultTags != nil && resultTags.Response.Response != nil && resultTags.StatusCode == http.StatusNotFound {
			fmt.Printf("%s repository not found\n", repoName)
			return nil, "", skippedTagsCount, processedMatchingTagsCount, nil
		}
		// An empty lastTag string is returned so there will not be any tag purged.
		return nil, "", skippedTagsCount, processedMatchingTagsCount, err
	}
	newLastTag := ""
	if resultTags != nil && resultTags.TagsAttributes != nil && len(*resultTags.TagsAttributes) > 0 {
		tags := *resultTags.TagsAttributes
		tagsEligibleForDeletion := []repository.TagToDelete{}
		for _, tag := range tags {
			matches, err = filter.MatchString(*tag.Name)
			if err != nil {
				// The only error that regexp2 will return is a timeout error
				return nil, "", skippedTagsCount, processedMatchingTagsCount, err
			}
			if !matches {
				continue
			}
			processedMatchingTagsCount++
			lastUpdateTime, err = time.Parse(time.RFC3339Nano, *tag.LastUpdateTime)
			if err != nil {
				return nil, "", skippedTagsCount, processedMatchingTagsCount, err
			}
			eligibleByAgeAndMinimum, eligibleByMaximum := repository.EvaluateRetention(lastUpdateTime, deleteCutoff, processedMatchingTagsCount, minTags, maxTags)
			if eligibleByAgeAndMinimum || eligibleByMaximum {
				reason := repository.DeletionReasonAge
				if eligibleByAgeAndMinimum && eligibleByMaximum {
					reason = repository.DeletionReasonAgeAndMaximumCount
				} else if eligibleByMaximum {
					reason = repository.DeletionReasonMaximumCount
				}
				if !includeLocked && tag.ChangeableAttributes != nil &&
					((tag.ChangeableAttributes.DeleteEnabled != nil && !*tag.ChangeableAttributes.DeleteEnabled) ||
						(tag.ChangeableAttributes.WriteEnabled != nil && !*tag.ChangeableAttributes.WriteEnabled)) {
					if minTags >= 0 || maxTags >= 0 {
						fmt.Printf("Warning: Retaining locked tag %s:%s (reason: %s)\n", repoName, *tag.Name, reason.String())
					}
					continue
				}
				tagsEligibleForDeletion = append(tagsEligibleForDeletion, repository.TagToDelete{
					TagAttributesBase: tag,
					Reason:            reason,
				})
			}
		}

		newLastTag = repository.GetLastTagFromResponse(resultTags)
		// No more tags to keep
		if keep == 0 || skippedTagsCount == keep {
			return tagsEligibleForDeletion, newLastTag, skippedTagsCount, processedMatchingTagsCount, nil
		}

		tagsToDelete := []repository.TagToDelete{}
		for _, tag := range tagsEligibleForDeletion {
			// Keep at least the configured number of tags
			if skippedTagsCount < keep {
				skippedTagsCount++
			} else {
				tagsToDelete = append(tagsToDelete, tag)
			}
		}
		return tagsToDelete, newLastTag, skippedTagsCount, processedMatchingTagsCount, nil
	}
	// In case there are no more tags return empty string as lastTag so that the purgeTags function stops
	return nil, "", skippedTagsCount, processedMatchingTagsCount, nil
}

// purgeDanglingManifests deletes all manifests that do not have any tags associated with them.
// except the ones that are referenced by a multiarch manifest or that have subject.
// If keep is provided, the specified number of most recent manifests will be kept.
func purgeDanglingManifests(ctx context.Context, acrClient api.AcrCLIClientInterface, repoParallelism int, loginURL string, repoName string, agoDuration *time.Duration, keep int, minManifests int, maxManifests int, manifestToTagsCountMap map[string]int, dryRun bool, includeLocked bool) (int, error) {
	if dryRun {
		fmt.Printf("Would delete manifests for repository: %s\n", repoName)
	} else {
		fmt.Printf("Deleting manifests for repository: %s\n", repoName)
	}
	// Without age or count limits, preserve legacy cleanup of all past manifests.
	timeToCompare := time.Now().UTC()
	if agoDuration != nil {
		timeToCompare = timeToCompare.Add(*agoDuration)
	}
	deleteCutoff := &timeToCompare
	if agoDuration == nil && maxManifests >= 0 {
		deleteCutoff = nil
	}
	// Contrary to getTagsToDelete, getManifestsToDelete gets all the Manifests at once, this was done because if there is a manifest that has no
	// tag but is referenced by a multiarch manifest that has tags then it should not be deleted. Or if a manifest has no tag, but it has subject,
	// then it should not be deleted.
	manifestsToDelete, err := repository.GetUntaggedManifests(ctx, repoParallelism, acrClient, repoName, false, manifestToTagsCountMap, dryRun, includeLocked, deleteCutoff, minManifests, maxManifests)
	if err != nil {
		return -1, err
	}

	// Apply keep logic if keep parameter is provided
	if keep > 0 {
		if len(manifestsToDelete) <= keep {
			return 0, nil
		}
		repository.SortManifestsByTime(manifestsToDelete)
		manifestsToDelete = manifestsToDelete[keep:]
	}

	// If dryRun is set to true then no manifests will be deleted, but the number of manifests that would be deleted is returned. Additionally,
	// the manifests that would be deleted are printed to the console. We also need to account for the manifests that would be deleted from the tag
	// filtering first as that would influence the untagged manifests that would be deleted.
	if dryRun {
		for _, manifest := range manifestsToDelete {
			reason := manifest.Reason
			if reason == repository.DeletionReasonUntagged && agoDuration != nil {
				reason = repository.DeletionReasonAge
			}
			if reasonText := reason.String(); reasonText != "" {
				fmt.Printf("Would delete: %s/%s@%s (reason: %s)\n", loginURL, repoName, *manifest.Digest, reasonText)
			} else {
				fmt.Printf("Would delete: %s/%s@%s\n", loginURL, repoName, *manifest.Digest)
			}
		}
		return len(manifestsToDelete), nil
	}
	// In order to only have a limited amount of http requests, a purger is used that will start goroutines to delete manifests.
	purger := worker.NewPurger(repoParallelism, acrClient, loginURL, repoName, includeLocked)
	manifestAttributes := make([]acr.ManifestAttributesBase, 0, len(manifestsToDelete))
	for _, manifest := range manifestsToDelete {
		manifestAttributes = append(manifestAttributes, manifest.ManifestAttributesBase)
	}
	deletedManifestsCount, purgeErr := purger.PurgeManifests(ctx, manifestAttributes)
	if purgeErr != nil {
		return -1, purgeErr
	}
	return deletedManifestsCount, nil
}

// isUnauthorizedError checks if an error is an HTTP 401 Unauthorized response.
// This is used to detect permission failures on ABAC-enabled registries where
// the user may have access to some repositories but not others.
func isUnauthorizedError(err error) bool {
	if err == nil {
		return false
	}
	var detailedErr autorest.DetailedError
	if errors.As(err, &detailedErr) {
		if statusCode, ok := detailedErr.StatusCode.(int); ok {
			return statusCode == http.StatusUnauthorized
		}
	}
	return strings.Contains(err.Error(), "StatusCode=401")
}

// formatPermissionError builds a clear error message when a purge operation fails
// due to insufficient permissions on a repository. It reports which repository
// failed, which repositories were already processed, and which remain untouched.
func formatPermissionError(failedRepo string, operation string, completedRepos []string, remainingRepos []string) error {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("insufficient permissions to %s for repository %q", operation, failedRepo))

	if len(completedRepos) > 0 {
		sb.WriteString(fmt.Sprintf("\n  Completed repositories (%d): %s", len(completedRepos), strings.Join(completedRepos, ", ")))
	} else {
		sb.WriteString("\n  Completed repositories: none")
	}

	// remainingRepos includes the failed repo; show the ones after it as not yet processed
	if len(remainingRepos) > 1 {
		sb.WriteString(fmt.Sprintf("\n  Remaining repositories not yet processed (%d): %s", len(remainingRepos)-1, strings.Join(remainingRepos[1:], ", ")))
	}

	sb.WriteString("\n  Hint: use a more specific --filter to target only repositories you have permissions for")
	return errors.New(sb.String())
}

// indexOf returns the index of s in slice, or 0 if not found.
func indexOf(slice []string, s string) int {
	for i, v := range slice {
		if v == s {
			return i
		}
	}
	return 0
}
