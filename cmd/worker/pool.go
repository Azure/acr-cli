package worker

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/Azure/acr-cli/cmd/api"
)

// pool manages a limited number of workers that process purgeJob.
type pool struct {
	size int
	sem  chan struct{}
}

// newPool creates a new pool.
func newPool(size int) *pool {
	return &pool{
		size: size,
		sem:  make(chan struct{}, size),
	}
}

// start starts a goroutine to process purgeJob.
func (p *pool) start(ctx context.Context, job purgeJob, acrClient api.AcrCLIClientInterface, errChan chan error, wg *sync.WaitGroup, succ *int64) {
	select {
	case <-ctx.Done():
		return
	case p.sem <- struct{}{}:
	}

	wg.Add(1)
	go func() {
		defer func() {
			<-p.sem
		}()
		defer wg.Done()

		err := job.process(ctx, acrClient)
		if err != nil {
			errChan <- err
		} else {
			atomic.AddInt64(succ, 1)
		}
	}()
}
