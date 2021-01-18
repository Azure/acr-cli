package worker

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/Azure/acr-cli/cmd/api"
	"golang.org/x/sync/semaphore"
)

// Pool uses semaphore to limit the number of goroutines processing PurgeJob.
type Pool struct {
	ctx     context.Context
	cancel  func()
	size    int
	sem     *semaphore.Weighted
	wg      sync.WaitGroup
	errChan chan error
	succ    int64
}

// NewPool creates a new Pool.
func NewPool(ctx context.Context, size int) *Pool {
	ctx, cancel := context.WithCancel(ctx)
	return &Pool{
		ctx:     ctx,
		cancel:  cancel,
		size:    size,
		sem:     semaphore.NewWeighted(int64(size)),
		errChan: make(chan error, 1),
	}
}

// Start starts a worker to handle PurgeJob.
// If error occurrs during processing job, it will be put into errChan and other workers will be canceled.
func (pool *Pool) Start(ctx context.Context, job purgeJob, acrClient api.AcrCLIClientInterface) {
	if err := pool.sem.Acquire(ctx, 1); err != nil {
		fmt.Printf("Failed to acquire semaphore: %v", err)
		return
	}
	pool.wg.Add(1)

	go func() {
		defer pool.sem.Release(1)

		// Check if there is error during processing job.
		if err := job.process(ctx, acrClient); err != nil {
			select {
			case pool.errChan <- err:
				// If err can be put into errChan, it means this is the first error occurred in the pool. Cancel other jobs.
				pool.cancel()
			default: // If err cannot be put into errChan, it means some previous error is already recorded. Ignore this one.
			}
		} else {
			// If there is no error, increase the success count.
			atomic.AddInt64(&pool.succ, 1)
		}

		pool.wg.Done()
	}()
}

//CheckError returns the error occurred in worker pool.
func (pool *Pool) CheckError() error {
	select {
	case err := <-pool.errChan:
		// If there is error recorded in the errChan, return that error.
		return err
	default:
		// If there is no error so far, return nil.
		return nil
	}
}

// SuccessCount returns the count of successful jobs in worker pool.
func (pool *Pool) SuccessCount() int {
	return int(atomic.LoadInt64(&pool.succ))
}

// Wait waits for all the workers in worker pool to finish.
func (pool *Pool) Wait() {
	pool.wg.Wait()
}
