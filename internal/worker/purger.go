// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package worker

import (
	"context"

	"github.com/Azure/acr-cli/acr"
	"github.com/Azure/acr-cli/internal/api"
)

// Purger purges tags or manifests concurrently.
type Purger struct {
	Executer
	acrClient api.AcrCLIClientInterface
}

// NewPurger creates a new Purger.
func NewPurger(poolSize int, acrClient api.AcrCLIClientInterface, loginURL string, repoName string) *Purger {
	executeBase := Executer{
		pool:     newPool(poolSize),
		loginURL: loginURL,
		repoName: repoName,
	}
	return &Purger{
		Executer:  executeBase,
		acrClient: acrClient,
	}
}

// PurgeTags purges a list of tags concurrently, and returns a count of deleted tags and the first error occurred.
func (p *Purger) PurgeTags(ctx context.Context, tags *[]acr.TagAttributesBase) (int, error) {
	jobs := make([]job, len(*tags))
	for i, tag := range *tags {
		jobs[i] = newPurgeTagJob(p.loginURL, p.repoName, p.acrClient, *tag.Name)
	}

	return p.process(ctx, &jobs)
}

// PurgeManifests purges a list of manifests concurrently, and returns a count of deleted manifests and the first error occurred.
func (p *Purger) PurgeManifests(ctx context.Context, manifests *[]string) (int, error) {
	jobs := make([]job, len(*manifests))
	for i, manifest := range *manifests {
		jobs[i] = newPurgeManifestJob(p.loginURL, p.repoName, p.acrClient, manifest)
	}

	return p.process(ctx, &jobs)
}
