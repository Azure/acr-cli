// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package worker

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"

	"github.com/Azure/acr-cli/acr"
	"github.com/Azure/acr-cli/internal/api"
	"github.com/alitto/pond/v2"
)

// Purger purges tags or manifests concurrently.
type Purger struct {
	Executer
	acrClient api.AcrCLIClientInterface
}

// NewPurger creates a new Purger. Purgers are currently repository specific
func NewPurger(poolSize int, acrClient api.AcrCLIClientInterface, loginURL string, repoName string) *Purger {
	executeBase := Executer{
		pool:     pond.NewPool(poolSize, pond.WithQueueSize(poolSize*3), pond.WithNonBlocking(false)),
		loginURL: loginURL,
		repoName: repoName,
	}
	return &Purger{
		Executer:  executeBase,
		acrClient: acrClient,
	}
}

// PurgeTags purges a list of tags concurrently, and returns a count of deleted tags and the first error occurred.
func (p *Purger) PurgeTags(ctx context.Context, tags []acr.TagAttributesBase) (int, error) {
	var deletedTags atomic.Int64 // Count of successfully deleted tags
	group := p.pool.NewGroup()
	for _, tag := range tags {
		group.SubmitErr(func() error {
			resp, err := p.acrClient.DeleteAcrTag(ctx, p.repoName, *tag.Name)
			if err == nil {
				fmt.Printf("Deleted %s/%s:%s\n", p.loginURL, p.repoName, *tag.Name)
				// Increment the count of successfully deleted tags atomically
				deletedTags.Add(1)
				return nil
			}

			if resp != nil && resp.Response != nil && resp.StatusCode == http.StatusNotFound {
				// If the tag is not found it can be assumed to have been deleted.
				deletedTags.Add(1)
				fmt.Printf("Skipped %s/%s:%s, HTTP status: %d\n", p.loginURL, p.repoName, *tag.Name, resp.StatusCode)
				return nil
			}

			fmt.Printf("Failed to delete %s/%s:%s, error: %v\n", p.loginURL, p.repoName, *tag.Name, err)
			return err
		})
	}
	err := group.Wait() // Error should be nil
	return int(deletedTags.Load()), err
}

// PurgeManifests purges a list of manifests concurrently, and returns a count of deleted manifests and the first error occurred.
func (p *Purger) PurgeManifests(ctx context.Context, manifests []string) (int, error) {
	var deletedManifests atomic.Int64 // Count of successfully deleted tags
	group := p.pool.NewGroup()
	for _, manifest := range manifests {
		group.SubmitErr(func() error {
			resp, err := p.acrClient.DeleteManifest(ctx, p.repoName, manifest)
			if err == nil {
				fmt.Printf("Deleted %s/%s:%s\n", p.loginURL, p.repoName, manifest)
				// Increment the count of successfully deleted tags atomically
				deletedManifests.Add(1)
				return nil
			}

			if resp != nil && resp.Response != nil && resp.StatusCode == http.StatusNotFound {
				// If the tag is not found it can be assumed to have been deleted.
				deletedManifests.Add(1)
				fmt.Printf("Skipped %s/%s:%s, HTTP status: %d\n", p.loginURL, p.repoName, manifest, resp.StatusCode)
				return nil
			}

			fmt.Printf("Failed to delete %s/%s:%s, error: %v\n", p.loginURL, p.repoName, manifest, err)
			return err

		})
	}
	err := group.Wait()
	return int(deletedManifests.Load()), err
}
