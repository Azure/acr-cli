// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package worker

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"github.com/Azure/acr-cli/cmd/api"
)

// PurgeWorker defines a worker that can process PurgeJobs.
type PurgeWorker struct {
	Job         chan PurgeJob
	WorkerQueue chan chan PurgeJob
	StopChan    chan bool
	wg          *sync.WaitGroup
	acrClient   api.AcrCLIClientInterface
}

// NewPurgeWorker creates a new worker.
func NewPurgeWorker(wg *sync.WaitGroup, workerQueue chan chan PurgeJob, acrClient api.AcrCLIClientInterface) PurgeWorker {
	worker := PurgeWorker{
		Job:         make(chan PurgeJob),
		StopChan:    make(chan bool),
		WorkerQueue: workerQueue,
		wg:          wg,
		acrClient:   acrClient,
	}
	return worker
}

// Start starts a new purgeWork with an infinite loop inside a goroutine.
func (pw *PurgeWorker) Start(ctx context.Context) {
	go func() {
		for {
			// Free worker, insert worker into worker queue.
			WorkerQueue <- pw.Job
			select {
			// If the worker has a job assigned begin processing it.
			case job := <-pw.Job:
				pw.ProcessJob(ctx, job)
			// If the worker needs to be stopped then return.
			case <-pw.StopChan:
				return
			}
		}
	}()
}

// Stop notifies the worker to stop handling purge jobs
func (pw *PurgeWorker) Stop() {
	pw.StopChan <- true
}

// ProcessJob processes any job (currently PurgeTag and PurgeManifest)
func (pw *PurgeWorker) ProcessJob(ctx context.Context, job PurgeJob) {
	go func() {
		defer pw.wg.Done()
		var wErr workerError
		switch job.JobType {
		case PurgeTag:
			// In case a tag is going to be purged DeleteAcrTag method is used.
			resp, err := pw.acrClient.DeleteAcrTag(ctx, job.RepoName, job.Tag)
			if err != nil {
				if resp != nil && resp.StatusCode == http.StatusNotFound {
					// If the tag is not found it can be assumed to have been deleted.
					fmt.Printf("Skipped %s/%s:%s, HTTP status: %d\n", job.LoginURL, job.RepoName, job.Tag, resp.StatusCode)
				} else {
					wErr = workerError{
						JobType: PurgeTag,
						Error:   err,
					}
				}
			} else {
				fmt.Printf("%s/%s:%s\n", job.LoginURL, job.RepoName, job.Tag)
			}
		case PurgeManifest:
			// In case a manifest is going to be purged DeleteManifest method is used.
			resp, err := pw.acrClient.DeleteManifest(ctx, job.RepoName, job.Digest)
			if err != nil {
				if resp != nil && resp.StatusCode == http.StatusNotFound {
					// If the manifest is not found it can be assumed to have been deleted.
					fmt.Printf("Skipped %s/%s@%s, HTTP status: %d\n", job.LoginURL, job.RepoName, job.Digest, resp.StatusCode)
				} else {
					wErr = workerError{
						JobType: PurgeTag,
						Error:   err,
					}
				}
			} else {
				fmt.Printf("%s/%s@%s\n", job.LoginURL, job.RepoName, job.Digest)
			}
		}
		ErrorChannel <- wErr
	}()
}
