package worker

import (
	"context"
	"fmt"

	"golang.org/x/sync/semaphore"
)

// workerPool uses semaphore to limit the number of goroutines processing PurgeJob.
type workerPool struct {
	worker    worker
	workerNum int64
	batchSize int
	sem       *semaphore.Weighted
	errChan   chan PurgeJobError
}

// worker defines a work function that handles PurgeJob and returns PurgeJobError.
type worker interface {
	work(context.Context, PurgeJob) PurgeJobError
}

// newWorkerPool creates a new workerPool.
func newWorkerPool(worker worker, workerNum int64, batchSize int) *workerPool {
	return &workerPool{
		worker:    worker,
		workerNum: workerNum,
		batchSize: batchSize,
		sem:       semaphore.NewWeighted(workerNum),
		errChan:   make(chan PurgeJobError, batchSize),
	}
}

// start starts a worker to handle PurgeJob.
func (wp *workerPool) start(ctx context.Context, job PurgeJob) {
	if err := wp.sem.Acquire(ctx, 1); err != nil {
		fmt.Printf("Failed to acquire semaphore: %v", err)
	}

	go func() {
		defer wp.sem.Release(1)
		wp.errChan <- wp.worker.work(ctx, job)
	}()
}

// wait waits for all the workers to finish.
func (wp *workerPool) wait(ctx context.Context) {
	if err := wp.sem.Acquire(ctx, wp.workerNum); err != nil {
		fmt.Printf("Failed to acquire semaphore: %v", err)
	}

	wp.sem.Release(wp.workerNum)
}
