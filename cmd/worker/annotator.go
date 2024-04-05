package worker

import (
	"context"
	"strings"
	"sync"

	"github.com/Azure/acr-cli/acr"
	"github.com/Azure/acr-cli/cmd/api"
	// "oras.land/oras-go/v2/registry/remote"
)

// Annotator annotates tags or manifests concurrently.
type Annotator struct {
	pool         *pool
	orasClient   api.ORASClientInterface
	loginURL     string
	repoName     string
	artifactType string
	annotations  map[string]string
}

// NewAnnotator creates a new Annotator.
func NewAnnotator(poolSize int, orasClient api.ORASClientInterface, loginURL string, repoName string, artifactType string, annotations []string) *Annotator {
	annotationsMap := convertListToMap(annotations)
	return &Annotator{
		pool: newPool(poolSize),
		// acrClient:    acrClient,
		orasClient:   orasClient,
		loginURL:     loginURL,
		repoName:     repoName,
		artifactType: artifactType,
		annotations:  annotationsMap,
	}
}

// process starts annotate jobs in worker pool and returns a count of successful jobs and the first error occurred.
func (a *Annotator) process(ctx context.Context, jobs *[]annotateJob) (int, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	var succ int64
	errChan := make(chan error)

	// Start purge jobs in worker pool.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for _, job := range *jobs {
			a.pool.startAnnotate(ctx, job, a.orasClient, errChan, &wg, &succ)
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

// AnnotateTags annotates a list of tags concurrently and returns a count of annotated tags and the first error occurred.
func (a *Annotator) AnnotateTags(ctx context.Context, tags *[]acr.TagAttributesBase) (int, error) {
	jobs := make([]annotateJob, len(*tags))
	for i, tag := range *tags {
		jobs[i] = newAnnotateTagJob(a.loginURL, a.repoName, a.artifactType, a.annotations, *tag.Name)
	}

	return a.process(ctx, &jobs)
}

// AnnotateManifests annotates a list of manifests concurrently and returns a count of annotated manifests and the first error occurred.
func (a *Annotator) AnnotateManifests(ctx context.Context, manifests *[]acr.ManifestAttributesBase) (int, error) {
	jobs := make([]annotateJob, len(*manifests))
	for i, manifest := range *manifests {
		jobs[i] = newAnnotateManifestJob(a.loginURL, a.repoName, a.artifactType, a.annotations, *manifest.Digest)
	}

	return a.process(ctx, &jobs)
}

// convertListToMap takes a list of annotations and converts it into a map, where the keys are the contents before the = and the values
// are the contents after the =. This is done so ORAS can be used to annotate.
func convertListToMap(annotations []string) map[string]string {
	// EOL annotation: "vnd.microsoft.artifact.end-of-life.date=2024-03-21"
	annotationMap := map[string]string{}
	for _, annotation := range annotations {
		// fmt.Println("annotation = ", annotation)
		arr := strings.Split(annotation, "=")
		annotationMap[arr[0]] = arr[1]
	}

	return annotationMap
}
