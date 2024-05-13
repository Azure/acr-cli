// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package worker

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type job interface {
	execute(context.Context) error
}

type jobBase struct {
	loginURL    string
	repoName    string
	timeCreated time.Time
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

// execute calls acrClient to annotate a manifest or tag.
func (job *annotateJob) execute(ctx context.Context) error {
	ref := fmt.Sprintf("%s/%s@%s", job.loginURL, job.repoName, job.ref)
	err := job.client.Annotate(ctx, ref, job.artifactType, job.annotations)
	if err == nil {
		fmt.Printf("Annotated %s/%s@%s\n", job.loginURL, job.repoName, job.ref)
		return nil
	}

	return err
}

// process starts jobs in worker pool, and returns a count of successful jobs and the first error occurred.
func (e *Executer) process(ctx context.Context, jobs *[]job) (int, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	var succ int64
	errChan := make(chan error)

	// Start purge jobs in worker pool.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for _, job := range *jobs {
			e.pool.start(ctx, job, errChan, &wg, &succ)
		}
	}()

	// Wait for all purge jobs to finish.
	go func() {
		wg.Wait()
		close(errChan)
	}()

	// If there are errors occurred during processing purge jobs, record the first error and cancel other jobs.
	var firstErr error
	for err := range errChan {
		if firstErr == nil {
			firstErr = err
			cancel()
		}
	}

	return int(succ), firstErr
}
