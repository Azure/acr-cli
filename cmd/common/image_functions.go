// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package common

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Azure/acr-cli/acr"
	"github.com/Azure/acr-cli/acr/acrapi"
	"github.com/Azure/acr-cli/internal/api"
	"github.com/dlclark/regexp2"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"golang.org/x/sync/errgroup"
)

const (
	headerLink                                            = "Link"
	mediaTypeDockerManifestList                           = "application/vnd.docker.distribution.manifest.list.v2+json"
	defaultRegexpOptions             regexp2.RegexOptions = regexp2.RE2 // This option will turn on compatibility mode so that it uses the group rules in regexp
	defaultRegexpMatchTimeoutSeconds int64                = 60
	mediaTypeArtifactManifest                             = "application/vnd.oci.artifact.manifest.v1+json"
)

func GetAllRepositoryNames(ctx context.Context, client acrapi.BaseClientAPI, pageSize int32) ([]string, error) {
	allRepoNames := make([]string, 0)
	lastName := ""
	for {
		repos, err := client.GetRepositories(ctx, lastName, &pageSize)
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

// GetMatchingRepos get all repositories in current registry, that match the provided regular expression
func GetMatchingRepos(repoNames []string, repoRegex string, regexMatchTimeout int64) ([]string, error) {
	filter, err := BuildRegexFilter(repoRegex, regexMatchTimeout)
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

// GetRepositoryAndTagRegex splits the strings that are in the form <repository>:<regex filter>
func GetRepositoryAndTagRegex(filter string) (string, string, error) {
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

// CollectTagFilters collects all matching repos and collects the associated tag filters
func CollectTagFilters(ctx context.Context, rawFilters []string, client acrapi.BaseClientAPI, regexMatchTimeout int64, repoPageSize int32) (map[string]string, error) {
	allRepoNames, err := GetAllRepositoryNames(ctx, client, repoPageSize)
	if err != nil {
		return nil, err
	}

	tagFilters := map[string]string{}
	for _, filter := range rawFilters {
		repoRegex, tagRegex, err := GetRepositoryAndTagRegex(filter)
		if err != nil {
			return nil, err
		}
		repoNames, err := GetMatchingRepos(allRepoNames, "^"+repoRegex+"$", regexMatchTimeout)
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

func GetLastTagFromResponse(resultTags *acr.RepositoryTagsType) string {
	// The lastTag is updated to keep the for loop going.
	if resultTags.Header == nil {
		return ""
	}
	link := resultTags.Header.Get(headerLink)
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

// GetUntaggedManifests gets all the manifests for the command to be executed on. The command will be executed on this manifest if it does not
// have any tag and does not form part of a manifest list that has tags referencing it. If the purge command is to be executed,
// the manifest should also not have a tag and not have a subject manifest.
func GetUntaggedManifests(ctx context.Context, acrClient api.AcrCLIClientInterface, loginURL string, repoName string, dryRun bool, dontPreserveAllOCIManifests bool) (*[]string, error) {
	lastManifestDigest := ""
	var manifestsToDelete []string
	resultManifests, err := acrClient.GetAcrManifests(ctx, repoName, "", lastManifestDigest)
	if err != nil {
		if resultManifests != nil && resultManifests.Response.Response != nil && resultManifests.StatusCode == http.StatusNotFound {
			fmt.Printf("%s repository not found\n", repoName)
			return &manifestsToDelete, nil
		}
		return nil, err
	}

	// This will act as a set. If a key is present, then the command shouldn't be executed because it is referenced by a multiarch manifest
	// or the manifest has subjects attached
	// #BUG add concurrency guards to this
	ignoreList := sync.Map{}

	// We will be adding to this map concurrently, so we need to use a mutex to lock it
	var candidates = make(map[string]acr.ManifestAttributesBase)

	// Read operations, specifically manifest gets are less throttled and so we can do more at once
	errgroup, ctx := errgroup.WithContext(ctx)
	errgroup.SetLimit(10)

	for resultManifests != nil && resultManifests.ManifestsAttributes != nil {
		manifests := *resultManifests.ManifestsAttributes
		for _, manifest := range manifests {
			// In the rare event that we run into an error with the errgroup while still doing the manifest acquisition loop,
			// we need to check if the context is done to break out of the loop early.
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}

			if manifest.Digest == nil {
				continue
			}

			// Check if the manifest is already in the ignoreList and can be skipped
			if _, ok := ignoreList.Load(*manifest.Digest); ok {
				continue
			}

			// _____MANIFEST HAS DELETION AS DISALLOWED BY ATTRIBUTES_____
			// If the manifest cannot be deleted or written to we can skip them (ACR will not allow deletion of these manifests)
			if manifest.ChangeableAttributes != nil {
				if manifest.ChangeableAttributes.DeleteEnabled != nil && *manifest.ChangeableAttributes.DeleteEnabled == false {
					continue
				}
				if manifest.ChangeableAttributes.WriteEnabled != nil && *manifest.ChangeableAttributes.WriteEnabled == false {
					continue
				}
			}

			// _____MANIFEST IS TAGGED_____
			// If an image has tags, it should not be deleted. If it is a manifest list, we need to check for its children which might be untagged
			if manifest.Tags != nil && len(*manifest.Tags) > 0 {
				// This case is a bug, out of an abundance of caution we will ignore it
				if manifest.MediaType == nil {
					continue
				}

				// If the manifest is not a list type, we can skip searching for its children
				if *manifest.MediaType != v1.MediaTypeImageIndex && *manifest.MediaType != mediaTypeDockerManifestList {
					continue
				}

				// Schedule a goroutine to find the dependent manifests and add them to the ignoreList
				errgroup.Go(func() error {
					manifests, err := FindDependentManifests(ctx, manifest, acrClient, repoName)
					if err != nil {
						return err
					}
					// Add all the manifests to the ignoreList
					for _, manifest := range manifests {
						ignoreList.LoadOrStore(manifest, struct{}{})
					}
					return nil
				})

				continue // We can skip the rest of the loop since the index is tagged and we are going to find its children
			}

			// _____MANIFEST IS UNTAGGED BUT MAY BE PROTECTED_____
			// TODO: I am a little unclear as to why this was ever an option but respecting it for now. Its not used by the purge scenarios.
			if !dontPreserveAllOCIManifests {
				if *manifest.MediaType != v1.MediaTypeImageManifest { // BUG? we also need to check for indexes today I think
					// Add the manifest to the candidates list
					if _, ok := candidates[*manifest.Digest]; !ok {
						candidates[*manifest.Digest] = manifest
					}
				}
				continue
			}

			// ______MANIFEST IS UNTAGGED BUT MAY STILL BE A REFERRER_____
			// If the manifest is a referrer type we want to preserve it as ACR does cleanup on the server side when a parent manifest is deleted

			// Schedule a goroutine to check if the manifest is okay to delete, it will be added to the
			// candidates list anyway but if it is not okay to delete we will add it to the ignoreList
			errgroup.Go(func() error {
				canDelete, err := IsManifestOkayToDelete(ctx, manifest, acrClient, repoName)
				if err != nil {
					return err
				}
				if canDelete {
					// Manifest is okay to delete
					return nil
				}
				if *manifest.MediaType == v1.MediaTypeImageIndex {
					// If the manifest is a multiarch manifest, we need to find its children manifests and add them to the ignoreList
					manifests, err := FindDependentManifests(ctx, manifest, acrClient, repoName)
					if err != nil {
						return err
					}
					// Add all the sub manifests to the ignoreList
					for _, manifest := range manifests {
						ignoreList.LoadOrStore(manifest, struct{}{})
					}
				}
				ignoreList.LoadOrStore(*manifest.Digest, struct{}{})
				return nil
			})

			// _____MANIFEST IS A CANDIDATE FOR DELETION_____

			// If we make it here we can add the manifest to the candidates map, it might still be marked as to ignore by the ignoreList
			// subsequently but that is not a problem.
			if _, ok := candidates[*manifest.Digest]; !ok {
				candidates[*manifest.Digest] = manifest
			}
		}

		// Get the last manifest digest from the last manifest from manifests.
		lastManifestDigest = *manifests[len(manifests)-1].Digest
		// Use this new digest to find next batch of manifests.
		resultManifests, err = acrClient.GetAcrManifests(ctx, repoName, "", lastManifestDigest)
		if err != nil {
			return nil, err
		}
	}

	// Wait for all the goroutines to finish
	if err := errgroup.Wait(); err != nil {
		return nil, err
	}

	// Remove everything from the candidates list that is in the ignoreList
	for _, manifest := range candidates {
		if _, ok := ignoreList.Load(*manifest.Digest); ok {
			// Remove the manifest from the candidates list
			delete(candidates, *manifest.Digest)
		}
	}

	for _, manifest := range candidates {
		// Add the manifest to the list of manifests to delete
		manifestsToDelete = append(manifestsToDelete, *manifest.Digest)
	}

	return &manifestsToDelete, nil
}

// FindDependentManifests adds the dependant manifest to doNotAffect if the referred manifest has tags.
// We also need to account for nested indexes. #BUG
func FindDependentManifests(ctx context.Context, manifest acr.ManifestAttributesBase, acrClient api.AcrCLIClientInterface, repoName string) ([]string, error) {
	var dependentManifestDigests []string
	switch *manifest.MediaType {
	case mediaTypeDockerManifestList, v1.MediaTypeImageIndex:
		var manifestBytes []byte
		manifestBytes, err := acrClient.GetManifest(ctx, repoName, *manifest.Digest)
		if err != nil {
			return nil, err
		}
		// this struct defines a customized struct for manifests
		// which is used to parse the content of a multiarch manifest
		subManifestOnlyStruct := struct {
			Manifests []v1.Descriptor `json:"manifests"`
		}{}

		if err = json.Unmarshal(manifestBytes, &subManifestOnlyStruct); err != nil {
			return nil, err
		}

		// Add all the manifests to the result
		dependentManifestDigests = make([]string, len(subManifestOnlyStruct.Manifests))
		for i, dependentManifest := range subManifestOnlyStruct.Manifests {
			dependentManifestDigests[i] = string(dependentManifest.Digest)
		}
	}
	return dependentManifestDigests, nil
}

// IsManifestOkayToDelete returns if a specific manifest is okay to delete. This depends on the following
// criteria:
// - Referrer manifests are not deleted (If a subject is present, the manifest is not deleted)
func IsManifestOkayToDelete(ctx context.Context, manifest acr.ManifestAttributesBase, acrClient api.AcrCLIClientInterface, repoName string) (bool, error) {
	switch *manifest.MediaType {
	case mediaTypeArtifactManifest, v1.MediaTypeImageManifest, v1.MediaTypeImageIndex:
		var manifestBytes []byte
		manifestBytes, err := acrClient.GetManifest(ctx, repoName, *manifest.Digest)
		if err != nil {
			return false, err
		}
		// this struct defines a customized struct for manifests which
		// is used to parse the content of a manifest references a subject
		subjectOnlyStruct := struct {
			Subject *v1.Descriptor `json:"subject,omitempty"`
		}{}
		if err = json.Unmarshal(manifestBytes, &subjectOnlyStruct); err != nil {
			return false, err
		}

		// Subject should be nil if the manifest does not contain a subject,
		// but add a check for the actual struct values just in case
		if subjectOnlyStruct.Subject != nil && subjectOnlyStruct.Subject.Digest != "" {
			return false, nil
		} else { // No subject means the manifest is not a referrer type
			return true, nil
		}
	default:
		return true, nil // This means the manifest is not a referrer type and can be deleted
	}
}

// BuildRegexFilter compiles a regex state machine from a regex expression
func BuildRegexFilter(expression string, regexpMatchTimeoutSeconds int64) (*regexp2.Regexp, error) {
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
