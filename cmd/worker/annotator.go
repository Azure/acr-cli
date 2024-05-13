package worker

import (
	"context"
	"errors"
	"strings"

	"github.com/Azure/acr-cli/cmd/api"
)

type Executer struct {
	pool     *pool
	loginURL string
	repoName string
}

// Annotator annotates tags or manifests concurrently.
type Annotator struct {
	Executer
	orasClient   api.ORASClientInterface
	artifactType string
	annotations  map[string]string
}

// NewAnnotator creates a new Annotator.
func NewAnnotator(poolSize int, orasClient api.ORASClientInterface, loginURL string, repoName string, artifactType string, annotations []string) (*Annotator, error) {
	annotationsMap, err := convertListToMap(annotations)
	if err != nil {
		return nil, err
	}
	executeBase := Executer{
		pool:     newPool(poolSize),
		loginURL: loginURL,
		repoName: repoName,
	}
	return &Annotator{
		Executer:     executeBase,
		orasClient:   orasClient,
		artifactType: artifactType,
		annotations:  annotationsMap,
	}, nil
}

// AnnotateTags annotates a list of digests (tags and manifests) concurrently and returns a count of annotated tags & manifests and the first error occurred.
func (a *Annotator) Annotate(ctx context.Context, digests *[]string) (int, error) {
	jobs := make([]job, len(*digests))
	for i, digest := range *digests {
		jobs[i] = newAnnotateJob(a.loginURL, a.repoName, a.artifactType, a.annotations, a.orasClient, digest)
	}

	return a.process(ctx, &jobs)
}

// convertListToMap takes a list of annotations and converts it into a map, where the keys are the contents before the = and the values
// are the contents after the =. This is done so ORAS can be used to annotate.
func convertListToMap(annotations []string) (map[string]string, error) {
	annotationMap := map[string]string{}
	for _, annotation := range annotations {
		arr := strings.Split(annotation, "=")
		if len(arr) == 1 {
			return nil, errors.New("annotation is not a key-value pair")
		}
		annotationMap[arr[0]] = arr[1]
	}

	return annotationMap, nil
}