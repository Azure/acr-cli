package worker

import (
	"context"
	"sync"

	"github.com/Azure/acr-cli/acr"
	"github.com/Azure/acr-cli/cmd/api"
)

type Lister struct {
	pool      *pool
	acrClient api.AcrCLIClientInterface
	loginURL  string
}

func NewLister(poolSize int, acrClient api.AcrCLIClientInterface, loginURL string) *Lister {
	return &Lister{
		pool:      newPool(poolSize),
		acrClient: acrClient,
		loginURL:  loginURL,
	}
}

func (p *Lister) process(ctx context.Context, jobs *[]Job) error {
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
			p.pool.start(ctx, job, p.acrClient, errChan, &wg, &succ)
		}
	}()

	// Wait for all jobs to finish.
	go func() {
		wg.Wait()
		close(errChan)
	}()

	// If there are errors occurred during processing jobs, record the first error and cancel other jobs.
	var firstErr error
	for err := range errChan {
		if firstErr == nil {
			firstErr = err
			cancel()
		}
	}

	return firstErr
}

func (p *Lister) ListRepos(ctx context.Context, repos *acr.Repositories) error {
	jobs := make([]Job, len(*repos.Names))
	for i, name := range *repos.Names {
		jobs[i] = newGetRepoSizeJob(p.loginURL, name)
	}

	return p.process(ctx, &jobs)
}