// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package worker

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/Azure/acr-cli/internal/api"
)

type purgeManifestJob struct {
	jobBase
	client api.AcrCLIClientInterface
	digest string
}

type purgeTagJob struct {
	jobBase
	client api.AcrCLIClientInterface
	tag    string
}

func newPurgeManifestJob(loginURL string, repoName string, client api.AcrCLIClientInterface, digest string) *purgeManifestJob {
	base := jobBase{
		loginURL:    loginURL,
		repoName:    repoName,
		timeCreated: time.Now().UTC(),
	}

	return &purgeManifestJob{
		jobBase: base,
		client:  client,
		digest:  digest,
	}
}

func newPurgeTagJob(loginURL string, repoName string, client api.AcrCLIClientInterface, tag string) *purgeTagJob {
	base := jobBase{
		loginURL:    loginURL,
		repoName:    repoName,
		timeCreated: time.Now().UTC(),
	}

	return &purgeTagJob{
		jobBase: base,
		client:  client,
		tag:     tag,
	}
}

// execute calls acrClient to delete a manifest.
func (job *purgeManifestJob) execute(ctx context.Context) error {
	resp, err := job.client.DeleteManifest(ctx, job.repoName, job.digest)
	if err == nil {
		fmt.Printf("Deleted %s/%s@%s\n", job.loginURL, job.repoName, job.digest)
		return nil
	}

	if resp != nil && resp.Response != nil && resp.StatusCode == http.StatusNotFound {
		// If the manifest is not found it can be assumed to have been deleted.
		fmt.Printf("Skipped %s/%s@%s, HTTP status: %d\n", job.loginURL, job.repoName, job.digest, resp.StatusCode)
		return nil
	}

	return err
}

// execute calls acrClient to delete a tag.
func (job *purgeTagJob) execute(ctx context.Context) error {
	resp, err := job.client.DeleteAcrTag(ctx, job.repoName, job.tag)
	if err == nil {
		fmt.Printf("Deleted %s/%s:%s\n", job.loginURL, job.repoName, job.tag)
		return nil
	}

	if resp != nil && resp.Response != nil && resp.StatusCode == http.StatusNotFound {
		// If the tag is not found it can be assumed to have been deleted.
		fmt.Printf("Skipped %s/%s:%s, HTTP status: %d\n", job.loginURL, job.repoName, job.tag, resp.StatusCode)
		return nil
	}

	return err
}
