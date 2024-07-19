// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package worker

import (
	"context"
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

// process starts jobs in worker pool, and returns a count of successful jobs and the first error occurred.
func (e *Executer) process(ctx context.Context, jobs *[]job) (int, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	var succ int64
	errChan := make(chan error)

	// Start jobs in worker pool.
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
