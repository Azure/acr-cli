package worker

import (
	"context"
	"fmt"
	"net/http"

	"github.com/Azure/acr-cli/cmd/api"
)

// purgeWorker leverages an acrClient to process purge jobs.
type purgeWorker struct {
	acrClient api.AcrCLIClientInterface
}

// newPurgeWorker creates a purgeWorker.
func newPurgeWorker(acrClient api.AcrCLIClientInterface) *purgeWorker {
	return &purgeWorker{
		acrClient: acrClient,
	}
}

// work processes purge jobs (currently PurgeTag and PurgeManifest).
func (pw *purgeWorker) work(ctx context.Context, job PurgeJob) (wErr PurgeJobError) {
	switch job.JobType {
	case PurgeTag:
		// In case a tag is going to be purged DeleteAcrTag method is used.
		resp, err := pw.acrClient.DeleteAcrTag(ctx, job.RepoName, job.Tag)
		if err != nil {
			if resp != nil && resp.Response != nil && resp.StatusCode == http.StatusNotFound {
				// If the tag is not found it can be assumed to have been deleted.
				fmt.Printf("Skipped %s/%s:%s, HTTP status: %d\n", job.LoginURL, job.RepoName, job.Tag, resp.StatusCode)
			} else {
				wErr = PurgeJobError{
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
			if resp != nil && resp.Response != nil && resp.StatusCode == http.StatusNotFound {
				// If the manifest is not found it can be assumed to have been deleted.
				fmt.Printf("Skipped %s/%s@%s, HTTP status: %d\n", job.LoginURL, job.RepoName, job.Digest, resp.StatusCode)
			} else {
				wErr = PurgeJobError{
					JobType: PurgeTag,
					Error:   err,
				}
			}
		} else {
			fmt.Printf("%s/%s@%s\n", job.LoginURL, job.RepoName, job.Digest)
		}
	}
	return
}
