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
	"time"

	"github.com/Azure/acr-cli/acr"
	"github.com/Azure/acr-cli/acr/acrapi"
	"github.com/Azure/acr-cli/internal/api"
	"github.com/Azure/acr-cli/internal/container/set"
	"github.com/dlclark/regexp2"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

const (
	headerLink                                            = "Link"
	mediaTypeDockerManifestList                           = "application/vnd.docker.distribution.manifest.list.v2+json"
	defaultRegexpOptions             regexp2.RegexOptions = regexp2.RE2 // This option will turn on compatibility mode so that it uses the group rules in regexp
	defaultRegexpMatchTimeoutSeconds uint64               = 60
	mediaTypeArtifactManifest                             = "application/vnd.oci.artifact.manifest.v1+json"
)

func GetAllRepositoryNames(ctx context.Context, client acrapi.BaseClientAPI) ([]string, error) {
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

// GetMatchingRepos get all repositories in current registry, that match the provided regular expression
func GetMatchingRepos(repoNames []string, repoRegex string, regexMatchTimeout uint64) ([]string, error) {
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
func CollectTagFilters(ctx context.Context, rawFilters []string, client acrapi.BaseClientAPI, regexMatchTimeout uint64) (map[string]string, error) {
	allRepoNames, err := GetAllRepositoryNames(ctx, client)
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
func GetUntaggedManifests(ctx context.Context, acrClient api.AcrCLIClientInterface, loginURL string, repoName string, dryRun bool, ignoreReferrerManifests bool) (*[]string, error) {
	lastManifestDigest := ""
	var manifestsForCommand []string
	resultManifests, err := acrClient.GetAcrManifests(ctx, repoName, "", lastManifestDigest)
	if err != nil {
		if resultManifests != nil && resultManifests.Response.Response != nil && resultManifests.StatusCode == http.StatusNotFound {
			fmt.Printf("%s repository not found\n", repoName)
			return &manifestsForCommand, nil
		}
		return nil, err
	}

	// This will act as a set. If a key is present, then the command shouldn't be executed because it is referenced by a multiarch manifest
	// or the manifest has subjects attached
	ignoreList := set.New[string]()
	var candidates []acr.ManifestAttributesBase
	for resultManifests != nil && resultManifests.ManifestsAttributes != nil {
		manifests := *resultManifests.ManifestsAttributes
		for _, manifest := range manifests {
			if manifest.Tags != nil {
				// If a manifest has Tags and its media type supports multiarch manifest, we will
				// iterate all its dependent manifests and mark them to not have the command execute on them.
				if err = AddDependentManifestsToIgnoreList(ctx, manifest, ignoreList, acrClient, repoName); err != nil {
					return nil, err
				}
			} else {
				if ignoreReferrerManifests {
					// If a manifest does not have Tags and its media type supports subject, we will
					// check if the subject exists. If so, the manifest is marked not to be affected by the command.
					if candidates, err = UpdateForManifestWithoutSubjectToDelete(ctx, manifest, ignoreList, candidates, acrClient, repoName); err != nil {
						return nil, err
					}
				} else {
					if *manifest.MediaType != v1.MediaTypeImageManifest {
						candidates = append(candidates, manifest)
					}
				}
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
	// Remove all manifests that should not be deleted
	for i := 0; i < len(candidates); i++ {
		if !ignoreList.Contains(*candidates[i].Digest) {
			// if a manifest has no tags, is not part of a manifest list and can be deleted then it is added to the
			// manifestsForCommand array.
			if *(candidates[i].ChangeableAttributes).DeleteEnabled && *(candidates[i].ChangeableAttributes).WriteEnabled {
				manifestsForCommand = append(manifestsForCommand, *candidates[i].Digest)
				if dryRun && !ignoreReferrerManifests {
					fmt.Printf("%s/%s@%s\n", loginURL, repoName, *candidates[i].Digest)
				}
			}
		}
	}

	return &manifestsForCommand, nil
}

// AddDependentManifestsToIgnoreList adds the dependant manifest to doNotAffect if the referred manifest has tags.
func AddDependentManifestsToIgnoreList(ctx context.Context, manifest acr.ManifestAttributesBase, doNotAffect set.Set[string], acrClient api.AcrCLIClientInterface, repoName string) error {
	switch *manifest.MediaType {
	case mediaTypeDockerManifestList, v1.MediaTypeImageIndex:
		var manifestBytes []byte
		manifestBytes, err := acrClient.GetManifest(ctx, repoName, *manifest.Digest)
		if err != nil {
			return err
		}
		// this struct defines a customized struct for manifests
		// which is used to parse the content of a multiarch manifest
		mam := struct {
			Manifests []v1.Descriptor `json:"manifests"`
		}{}

		if err = json.Unmarshal(manifestBytes, &mam); err != nil {
			return err
		}
		for _, dependentManifest := range mam.Manifests {
			doNotAffect.Add(dependentManifest.Digest.String())
		}
	}
	return nil
}

// UpdateForManifestWithoutSubjectToDelete adds the manifest to candidatesToDelete
// if the manifest does not have subject, otherwise add it to doNotDelete.
func UpdateForManifestWithoutSubjectToDelete(ctx context.Context, manifest acr.ManifestAttributesBase, doNotDelete set.Set[string], candidatesToDelete []acr.ManifestAttributesBase, acrClient api.AcrCLIClientInterface, repoName string) ([]acr.ManifestAttributesBase, error) {
	switch *manifest.MediaType {
	case mediaTypeArtifactManifest, v1.MediaTypeImageManifest, v1.MediaTypeImageIndex:
		var manifestBytes []byte
		manifestBytes, err := acrClient.GetManifest(ctx, repoName, *manifest.Digest)
		if err != nil {
			return nil, err
		}
		// this struct defines a customized struct for manifests which
		// is used to parse the content of a manifest references a subject
		mws := struct {
			Subject *v1.Descriptor `json:"subject,omitempty"`
		}{}
		if err = json.Unmarshal(manifestBytes, &mws); err != nil {
			return nil, err
		}
		if mws.Subject != nil {
			doNotDelete.Add(*manifest.Digest)
		} else {
			candidatesToDelete = append(candidatesToDelete, manifest)
		}
	default:
		candidatesToDelete = append(candidatesToDelete, manifest)
	}
	return candidatesToDelete, nil
}

// BuildRegexFilter compiles a regex state machine from a regex expression
func BuildRegexFilter(expression string, regexpMatchTimeoutSeconds uint64) (*regexp2.Regexp, error) {
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
