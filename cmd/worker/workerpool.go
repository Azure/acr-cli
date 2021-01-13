package worker

import (
	"context"
	"fmt"
	"sync"

	"github.com/Azure/acr-cli/cmd/api"
	"golang.org/x/sync/semaphore"
)

// workerPool uses semaphore to limit the number of goroutines processing PurgeJob.
type workerPool struct {
	workerNum int
	batchSize int
	sem       *semaphore.Weighted
	errChan   chan PurgeJobError
}

// newWorkerPool creates a new workerPool.
func newWorkerPool(workerNum int, batchSize int) *workerPool {
	return &workerPool{
		workerNum: workerNum,
		batchSize: batchSize,
		sem:       semaphore.NewWeighted(int64(workerNum)),
		errChan:   make(chan PurgeJobError, batchSize),
	}
}

// start starts a worker to handle PurgeJob.
func (pool *workerPool) start(ctx context.Context, job PurgeJob, acrClient api.AcrCLIClientInterface, wg *sync.WaitGroup) {
	if err := pool.sem.Acquire(ctx, 1); err != nil {
		fmt.Printf("Failed to acquire semaphore: %v", err)
		return
	}
	wg.Add(1)

	go func() {
		defer pool.sem.Release(1)
		pool.errChan <- job.Process(ctx, acrClient)
		wg.Done()
	}()
}
