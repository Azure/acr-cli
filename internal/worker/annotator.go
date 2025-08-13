// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

// Package worker provides concurrent workers for container registry operations.
package worker

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/Azure/acr-cli/internal/api"
	"github.com/alitto/pond/v2"
)

// Executer provides the base functionality for concurrent task execution.
type Executer struct {
	pool     pond.Pool
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
		// Use a queue size 3x the pool size to buffer enough tasks and keep workers busy and avoiding
		// slowdown due to task scheduling blocking.
		pool:     pond.NewPool(poolSize, pond.WithQueueSize(poolSize*3), pond.WithNonBlocking(false)),
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

// Annotate annotates a list of manifests concurrently and returns a count of annotated images and the first error occurred.
func (a *Annotator) Annotate(ctx context.Context, manifests []string) (int, error) {
	var annotatedImages atomic.Int64
	group := a.pool.NewGroup()

	for _, digest := range manifests {
		group.SubmitErr(func() error {
			ref := fmt.Sprintf("%s/%s@%s", a.loginURL, a.repoName, digest)
			if err := a.orasClient.Annotate(ctx, ref, a.artifactType, a.annotations); err != nil {
				fmt.Printf("Failed to annotate %s/%s@%s, error: %v\n", a.loginURL, a.repoName, digest, err)
				return err // TODO: #469 Do we want to fail the whole job if one fails? This is the current behaviour.
			}
			annotatedImages.Add(1)
			fmt.Printf("Annotated %s/%s@%s\n", a.loginURL, a.repoName, digest)
			return nil
		})
	}
	err := group.Wait()
	return int(annotatedImages.Load()), err
}

// convertListToMap takes a list of annotations and converts it into a map, where the keys are the contents before the = and the values
// are the contents after the =. This is done so ORAS can be used to annotate.
// Example: If the annotation is "vnd.microsoft.artifact.lifecycle.end-of-life-date=2024-06-17" , this function will return a map that
// looks like ["vnd.microsoft.artifact.lifecycle.end-of-life-date": "2024-06-17"]
func convertListToMap(annotations []string) (map[string]string, error) {
	annotationMap := map[string]string{}
	for _, annotation := range annotations {
		before, after, found := strings.Cut(annotation, "=")
		if !found {
			return nil, errors.New("annotation is not a key-value pair")
		}
		annotationMap[before] = after
	}

	return annotationMap, nil
}
