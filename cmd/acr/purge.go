// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"time"

	"github.com/Azure/acr-cli/acr"
	"github.com/Azure/acr-cli/acr/acrapi"
	"github.com/Azure/acr-cli/cmd/api"
	"github.com/Azure/acr-cli/cmd/worker"
	"github.com/dlclark/regexp2"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
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
	maxPoolSize                      = 32 // The max number of parallel delete requests recommended by ACR server
	manifestListContentType          = "application/vnd.docker.distribution.manifest.list.v2+json"
	manifestOCIArtifactContentType   = "application/vnd.oci.artifact.manifest.v1+json"
	manifestOCIImageContentType      = "application/vnd.oci.image.manifest.v1+json"
	manifestOCIImageIndexContentType = "application/vnd.oci.image.index.v1+json"
	linkHeader                       = "Link"
)

var (
	defaultPoolSize        = runtime.GOMAXPROCS(0)
	concurrencyDescription = fmt.Sprintf("Number of concurrent purge tasks. Range: [1 - %d]", maxPoolSize)
	mediaTypes             = map[string]struct{}{manifestListContentType: {}, manifestOCIArtifactContentType: {}, manifestOCIImageContentType: {}, manifestOCIImageIndexContentType: {}}
)

// Default settings for regexp2
const (
	defaultRegexpOptions             regexp2.RegexOptions = regexp2.RE2 // This option will turn on compatibility mode so that it uses the group rules in regexp
	defaultRegexpMatchTimeoutSeconds uint64               = 60
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
		RunE: func(cmd *cobra.Command, args []string) error {
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
			tagFilters, err := collectTagFilters(ctx, purgeParams.filters, acrClient.AutorestClient, purgeParams.filterTimeout)
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

	tagRegex, err := buildRegexFilter(tagFilter, regexpMatchTimeoutSeconds)
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

// collectTagFilters collects all matching repos and collects the associated tag filters
func collectTagFilters(ctx context.Context, rawFilters []string, client acrapi.BaseClientAPI, regexMatchTimeout uint64) (map[string]string, error) {
	allRepoNames, err := getAllRepositoryNames(ctx, client)
	if err != nil {
		return nil, err
	}

	tagFilters := map[string]string{}
	for _, filter := range rawFilters {
		repoRegex, tagRegex, err := getRepositoryAndTagRegex(filter)
		if err != nil {
			return nil, err
		}
		repoNames, err := getMatchingRepos(allRepoNames, "^"+repoRegex+"$", regexMatchTimeout)
		if err != nil {
			return nil, err
		}
		for _, repoName := range repoNames {
			if _, ok := tagFilters[repoName]; ok {
				// To only iterate through a repo once a big regex filter is made of all the filters of a particular repo.
				tagFilters[repoName] = tagFilters[repoName] + "|" + tagRegex
			} else {
				tagFilters[repoName] = tagRegex
			}
		}
	}

	return tagFilters, nil
}

func getAllRepositoryNames(ctx context.Context, client acrapi.BaseClientAPI) ([]string, error) {
	allRepoNames := make([]string, 0)
	lastName := ""
	var batchSize int32 = 100
	for {
		repos, err := client.GetRepositories(ctx, lastName, &batchSize)
		if err != nil {
			return nil, err
		}
		if repos.Names == nil || len(*repos.Names) == 0 {
			break
		}
		allRepoNames = append(allRepoNames, *repos.Names...)
		lastName = allRepoNames[len(allRepoNames)-1]
	}
	return allRepoNames, nil
}

// getMatchingRepos get all repositories in current registry, that match the provided regular expression
func getMatchingRepos(repoNames []string, repoRegex string, regexMatchTimeout uint64) ([]string, error) {
	filter, err := buildRegexFilter(repoRegex, regexMatchTimeout)
	if err != nil {
		return nil, err
	}
	var matchedRepos []string
	for _, repo := range repoNames {
		matched, err := filter.MatchString(repo)
		if err != nil {
			// The only error regexp2 can throw is a timeout error
			return nil, err
		}

		if matched {
			matchedRepos = append(matchedRepos, repo)
		}
	}
	return matchedRepos, nil
}

// getRepositoryAndTagRegex splits the strings that are in the form <repository>:<regex filter>
func getRepositoryAndTagRegex(filter string) (string, string, error) {
	// This only selects colons that are not apart of a non-capture group
	// Note: regexp2 doesn't have .Split support yet, so we just replace the colon with another delimitter \r\n
	// We choose \r\n since it is an escape sequence that cannot be a part of repo name or a tag
	// For information on how this expression was written, see https://regexr.com/6jqp3
	noncaptureGroupSupport := regexp2.MustCompile(`(?<!\(\?[imsU-]{0,5}|\[*\^*\[\^*):(?!\]\]*)`, defaultRegexpOptions)

	// Note: We could just find the first 1, however we want to know if there are more than 1 colon that is not part of a non-capture group
	newlineDelimitted, err := noncaptureGroupSupport.Replace(filter, "\r\n", -1, -1)
	if err != nil {
		return "", "", errors.New("could not replace split filter by repo and tag")
	}

	repoAndRegex := strings.Split(newlineDelimitted, "\r\n")
	if len(repoAndRegex) != 2 {
		return "", "", errors.New("unable to correctly parse filter flag")
	}

	if repoAndRegex[0] == "" {
		return "", "", errors.New("missing repository name/expression")
	}
	if repoAndRegex[1] == "" {
		return "", "", errors.New("missing tag name/expression")
	}
	return repoAndRegex[0], repoAndRegex[1], nil
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

		newLastTag = getLastTagFromResponse(resultTags)
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

func getLastTagFromResponse(resultTags *acr.RepositoryTagsType) string {
	// The lastTag is updated to keep the for loop going.
	if resultTags.Header == nil {
		return ""
	}
	link := resultTags.Header.Get(linkHeader)
	if len(link) == 0 {
		return ""
	}
	queryString := strings.Split(link, "?")
	if len(queryString) <= 1 {
		return ""
	}
	queryStringToParse := strings.Split(queryString[1], ">")
	vals, err := url.ParseQuery(queryStringToParse[0])
	if err != nil {
		return ""
	}
	return vals.Get("last")
}

// purgeDanglingManifests deletes all manifests that do not have any tags associated with them.
func purgeDanglingManifests(ctx context.Context, acrClient api.AcrCLIClientInterface, poolSize int, loginURL string, repoName string) (int, error) {
	fmt.Printf("Deleting manifests for repository: %s\n", repoName)
	// Contrary to getTagsToDelete, getManifestsToDelete gets all the Manifests at once, this was done because if there is a manifest that has no
	// tag but is referenced by a multiarch manifest that has tags then it should not be deleted.
	manifestsToDelete, err := getManifestsToDelete(ctx, acrClient, repoName)
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

// getManifestsToDelete gets all the manifests that should be deleted, this means that do not have any tag and that do not form part
// of a manifest list that has tags referencing it.
func getManifestsToDelete(ctx context.Context, acrClient api.AcrCLIClientInterface, repoName string) (*[]acr.ManifestAttributesBase, error) {
	lastManifestDigest := ""
	var manifestsToDelete []acr.ManifestAttributesBase
	resultManifests, err := acrClient.GetAcrManifests(ctx, repoName, "", lastManifestDigest)
	if err != nil {
		if resultManifests != nil && resultManifests.Response.Response != nil && resultManifests.StatusCode == http.StatusNotFound {
			fmt.Printf("%s repository not found\n", repoName)
			return &manifestsToDelete, nil
		}
		return nil, err
	}
	// This will act as a set if a key is present then it should not be deleted because it is referenced by a multiarch manifest
	// or the manifest has subjects attached
	doNotDelete := make(map[string]struct{})
	var candidatesToDelete []acr.ManifestAttributesBase
	for resultManifests != nil && resultManifests.ManifestsAttributes != nil {
		manifests := *resultManifests.ManifestsAttributes
		for _, manifest := range manifests {
			if _, ok := mediaTypes[*manifest.MediaType]; ok {
				manifestBytes, err := acrClient.GetManifest(ctx, repoName, *manifest.Digest)
				if err != nil {
					return nil, err
				}
				var customManifest customManifest
				err = json.Unmarshal(manifestBytes, &customManifest)
				if err != nil {
					return nil, err
				}
				if customManifest.Subject != nil {
					// if manifest has subject, manifest is marked to not be deleted
					doNotDelete[*manifest.Digest] = struct{}{}
				} else if manifest.Tags == nil {
					// If the manifest has no subject or tag, it is a candidate for deletion
					candidatesToDelete = append(candidatesToDelete, manifest)
				}
				if len(customManifest.Manifests) > 0 {
					// if manifest is a multiarchitecture manifest
					if manifest.Tags != nil {
						// If this multiarchitecture manifest has tag, its dependent
						// manifests are maked to not be deleted
						for _, dependentManifest := range customManifest.Manifests {
							doNotDelete[dependentManifest.Digest] = struct{}{}
						}
					} else {
						// If the manifest has no tag, it is a candidate for deletion
						candidatesToDelete = append(candidatesToDelete, manifest)
					}
				}
			} else {
				if manifest.Tags == nil {
					candidatesToDelete = append(candidatesToDelete, manifest)
				}
			}
		}
		lastManifestDigest = *manifests[len(manifests)-1].Digest
		resultManifests, err = acrClient.GetAcrManifests(ctx, repoName, "", lastManifestDigest)
		if err != nil {
			return nil, err
		}
	}
	// Remove all manifests that should not be deleted
	for i := 0; i < len(candidatesToDelete); i++ {
		if _, ok := doNotDelete[*candidatesToDelete[i].Digest]; !ok {
			// if a manifest has no tags, is not part of a manifest list and can be deleted then it is added to the
			// manifestToDelete array.
			if *(*candidatesToDelete[i].ChangeableAttributes).DeleteEnabled && *(*candidatesToDelete[i].ChangeableAttributes).WriteEnabled {
				manifestsToDelete = append(manifestsToDelete, candidatesToDelete[i])
			}
		}
	}
	return &manifestsToDelete, nil
}

// dryRunPurge outputs everything that would be deleted if the purge command was executed
func dryRunPurge(ctx context.Context, acrClient api.AcrCLIClientInterface, loginURL string, repoName string, ago string, filter string, untagged bool, keep int, regexMatchTimeout uint64) (int, int, error) {
	deletedTagsCount := 0
	deletedManifestsCount := 0
	// In order to keep track if a manifest would get deleted a map is defined that as a  key has the manifest
	// digest and as the value the number of tags (referencing said manifests) that were deleted.
	deletedTags := map[string]int{}
	fmt.Printf("Tags for this repository would be deleted: %s\n", repoName)
	agoDuration, err := parseDuration(ago)
	if err != nil {
		return -1, -1, err
	}
	timeToCompare := time.Now().UTC()
	timeToCompare = timeToCompare.Add(agoDuration)
	regex, err := buildRegexFilter(filter, regexMatchTimeout)
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
		// The countMap contains a map that for every digest contains how many tags are referencing it.
		countMap, err := countTagsByManifest(ctx, acrClient, repoName)
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
		// This will act as a set if a key is present then it should not be deleted
		// because it is referenced by a multiarch manifest or a manifest has subject attached
		// that will not be deleted
		doNotDelete := make(map[string]struct{})
		candidatesToDelete := []acr.ManifestAttributesBase{}
		// Iterate over all manifests to discover multiarchitecture manifests
		for resultManifests != nil && resultManifests.ManifestsAttributes != nil {
			manifests := *resultManifests.ManifestsAttributes
			for _, manifest := range manifests {
				if (*countMap)[*manifest.Digest] != deletedTags[*manifest.Digest] {
					if _, ok := mediaTypes[*manifest.MediaType]; ok {
						manifestBytes, err := acrClient.GetManifest(ctx, repoName, *manifest.Digest)
						if err != nil {
							return -1, -1, err
						}
						var customManifest customManifest
						err = json.Unmarshal(manifestBytes, &customManifest)
						if err != nil {
							return -1, -1, err
						}
						if customManifest.Subject != nil {
							// if manifest has subject, manifest is marked to not be deleted
							doNotDelete[*manifest.Digest] = struct{}{}
						} else if manifest.Tags == nil {
							// If the manifest has no subject or tag, it is a candidate for deletion
							candidatesToDelete = append(candidatesToDelete, manifest)
						}
						if len(customManifest.Manifests) > 0 {
							for _, dependentManifest := range customManifest.Manifests {
								doNotDelete[dependentManifest.Digest] = struct{}{}
							}
						}
					}
				} else {
					// If the manifest has the same amount of tags as the amount of tags deleted then it is a candidate for deletion.
					candidatesToDelete = append(candidatesToDelete, manifest)
				}
				lastManifestDigest = *manifests[len(manifests)-1].Digest
				resultManifests, err = acrClient.GetAcrManifests(ctx, repoName, "", lastManifestDigest)
				if err != nil {
					return -1, -1, err
				}
			}
		}
		// Just print manifests that would be deleted.
		for i := 0; i < len(candidatesToDelete); i++ {
			if _, ok := doNotDelete[*candidatesToDelete[i].Digest]; !ok {
				fmt.Printf("%s/%s@%s\n", loginURL, repoName, *candidatesToDelete[i].Digest)
				deletedManifestsCount++
			}
		}
	}

	return deletedTagsCount, deletedManifestsCount, nil
}

// countTagsByManifest returns a map that for a given manifest digest contains the number of tags associated to it.
func countTagsByManifest(ctx context.Context, acrClient api.AcrCLIClientInterface, repoName string) (*map[string]int, error) {
	countMap := map[string]int{}
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
			countMap[*tag.Digest]++
		}

		lastTag = *tags[len(tags)-1].Name
		// Keep on iterating until the resultTags or resultTags.TagsAttributes is nil
		resultTags, err = acrClient.GetAcrTags(ctx, repoName, "", lastTag)
		if err != nil {
			return nil, err
		}
	}
	return &countMap, nil
}

// buildRegexFilter compiles a regex state machine from a regex expression
func buildRegexFilter(expression string, regexpMatchTimeoutSeconds uint64) (*regexp2.Regexp, error) {
	regexp, err := regexp2.Compile(expression, defaultRegexpOptions)
	if err != nil {
		return nil, err
	}

	// A timeout value must always be set
	if regexpMatchTimeoutSeconds <= 0 {
		regexpMatchTimeoutSeconds = defaultRegexpMatchTimeoutSeconds
	}
	regexp.MatchTimeout = time.Duration(regexpMatchTimeoutSeconds) * time.Second

	return regexp, nil
}

// In order to parse the content of a mutliarch manifest string
// or a manifest contains subjects, the following structs were defined.
type customManifest struct {
	Manifests     []manifest     `json:"manifests"`
	MediaType     string         `json:"mediaType"`
	SchemaVersion int            `json:"schemaVersion"`
	Subject       *v1.Descriptor `json:"subject,omitempty"`
}

type manifest struct {
	Digest    string   `json:"digest"`
	MediaType string   `json:"mediaType"`
	Platform  platform `json:"platform"`
	Size      int64    `json:"size"`
}

type platform struct {
	Architecture string `json:"architecture"`
	Os           string `json:"os"`
}
