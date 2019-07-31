// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package worker

import (
	"sync"

	"github.com/Azure/acr-cli/cmd/api"
)

// WorkerQueue represents the queue of the workers
var WorkerQueue chan chan PurgeJob

// StartDispatcher creates the workers and a goroutine to continously fetch jobs for them.
func StartDispatcher(wg *sync.WaitGroup, acrClient api.AcrCLIClientInterface, nWorkers int) {
	WorkerQueue = make(chan chan PurgeJob, nWorkers)
	for i := 0; i < nWorkers; i++ {
		worker := NewPurgeWorker(wg, WorkerQueue, acrClient)
		worker.Start()
	}

	go func() {
		for job := range JobQueue {
			// Get a job from the JobQueue
			go func(job PurgeJob) {
				// Get a worker (block if there are none available) to process the job
				worker := <-WorkerQueue
				// Assign the job to the worker
				worker <- job
			}(job)
		}
	}()
}
