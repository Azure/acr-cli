// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/Azure/acr-cli/cmd/api"
)

type Job interface {
	process(context.Context, api.AcrCLIClientInterface) error
}

type repoSizeJob struct {
	loginURL    string
	repoName    string
	timeCreated time.Time
}

func newGetRepoSizeJob(loginURL string, repoName string) *repoSizeJob {
	return &repoSizeJob{
		loginURL: 	loginURL,
		repoName:   repoName,
	}
}

// process calls acrClient
func (job *repoSizeJob) process(ctx context.Context, acrClient api.AcrCLIClientInterface) error {
	var manifestSize int64 = 0
	lastManifestDigest := ""

	resultManifests, err := acrClient.GetAcrManifests(ctx, job.repoName, "", lastManifestDigest)
	if err != nil {
		return err
	}

	// A for loop is used because the GetAcrManifests method returns by default only 100 manifests and their attributes.
	for resultManifests != nil && resultManifests.ManifestsAttributes != nil {
		manifests := *resultManifests.ManifestsAttributes
		for _, manifest := range manifests {
			manifestSize += *manifest.ImageSize
		}
		// Since the GetAcrManifests supports pagination when supplied with the last digest that was returned the last manifest
		// digest is saved, the manifest array contains at least one element because if it was empty the API would return
		// a nil pointer instead of a pointer to a length 0 array.
		lastManifestDigest = *manifests[len(manifests)-1].Digest

		resultManifests, err = acrClient.GetAcrManifests(ctx, job.repoName, "", lastManifestDigest)
		if err != nil {
			return err
		}
	}

	manifestSizeInGB := float64(manifestSize)/1024/1024/1024
	fmt.Printf("%-50s%10.3f GB\n", job.repoName, manifestSizeInGB)

	return err
}
