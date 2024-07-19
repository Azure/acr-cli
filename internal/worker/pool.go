package worker

import (
	"context"
	"sync"
	"sync/atomic"
)

// pool manages a limited number of workers that process purgeJob.
type pool struct {
	sem chan struct{}
}

// newPool creates a new pool.
func newPool(size int) *pool {
	return &pool{
		sem: make(chan struct{}, size),
	}
}

// start starts a goroutine to process purgeJob.
func (p *pool) start(ctx context.Context, job job, errChan chan error, wg *sync.WaitGroup, succ *int64) {
	select {
	case <-ctx.Done():
		// Return when context is canceled
		return
	case p.sem <- struct{}{}: // Acquire a semaphore
	}

	wg.Add(1)
	go func() {
		// Release a semaphore
		defer func() {
			<-p.sem
		}()
		defer wg.Done()

		err := job.execute(ctx)
		// If error occurs, put error in errChan, otherwise increase the success count
		if err != nil {
			errChan <- err
		} else {
			atomic.AddInt64(succ, 1)
		}
	}()
}
