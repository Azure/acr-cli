// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package api

import (
	"context"
	"io"

	orasauth "github.com/Azure/acr-cli/auth/oras"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/file"
	"oras.land/oras-go/v2/registry"
	"oras.land/oras-go/v2/registry/remote"
)

const lifecycleAnnotation = "application/vnd.microsoft.artifact.lifecycle"

// The ORASClient wraps the oras-go sdk and is used for interacting with artifacts in a registry.
// it implements the ORASClientInterface.
type ORASClient struct {
	client remote.Client
}

// GraphTarget is a tracked oras.GraphTarget.
type GraphTarget interface {
	oras.GraphTarget
	io.Closer
	Prompt(desc ocispec.Descriptor, prompt string) error
	Inner() oras.GraphTarget
}

func (o *ORASClient) DiscoverLifecycleAnnotation(ctx context.Context, reference string, artifactType string) (bool, error) {
	ref, err := o.getTarget(reference)
	if err != nil {
		return false, err
	}
	subject, err := ref.Resolve(ctx, reference)
	if err != nil {
		return false, err
	}
	descriptors, err := registry.Referrers(ctx, ref, subject, artifactType)
	if err != nil {
		return false, err
	}

	for _, desc := range descriptors {
		if desc.ArtifactType == lifecycleAnnotation {
			return true, nil
		}
	}

	return false, nil
}

// type packFunc func() (ocispec.Descriptor, error)
// type copyFunc func(desc ocispec.Descriptor) error

func (o *ORASClient) Annotate(ctx context.Context, reference string, artifactType string, annotationsArg map[string]string) error {

	dst, err := o.getTarget(reference)
	if err != nil {
		return err
	}

	// do the equivalent of
	// prepare manifest
	store, err := file.New("")
	if err != nil {
		return err
	}
	defer store.Close()

	subject, err := dst.Resolve(ctx, reference)
	if err != nil {
		return err
	}

	graphCopyOptions := oras.DefaultCopyGraphOptions
	packOpts := oras.PackManifestOptions{
		Subject:             &subject,
		ManifestAnnotations: annotationsArg,
	}

	pack := func() (ocispec.Descriptor, error) {
		return oras.PackManifest(ctx, store, oras.PackManifestVersion1_1, artifactType, packOpts)
	}

	copyFunc := func(root ocispec.Descriptor) error {
		return oras.CopyGraph(ctx, store, dst, root, graphCopyOptions)
	}

	// Attach
	targetDst := oras.Target(dst)
	if tracked, ok := targetDst.(GraphTarget); ok {
		defer tracked.Close()
	}
	// Push
	root, err := pack()
	if err != nil {
		return err
	}

	if err = copyFunc(root); err != nil {
		return err
	}
	return nil
}

// getTarget gets an oras remote.Repository object that refers to the target of our annotation request
func (o *ORASClient) getTarget(reference string) (repo *remote.Repository, err error) {
	repo, err = remote.NewRepository(reference)
	if err != nil {
		return nil, err
	}

	repo.SkipReferrersGC = true
	repo.Client = o.client
	repo.SetReferrersCapability(true)
	return repo, nil
}

func GetORASClientWithAuth(username string, password string, configs []string) (*ORASClient, error) {
	clientOpts := orasauth.ClientOptions{}
	if username != "" && password != "" {
		clientOpts.Credential = orasauth.Credential(username, password)
	} else {
		store, err := orasauth.NewStore(configs...)
		if err != nil {
			return nil, err
		}
		clientOpts.CredentialStore = store
	}
	c := orasauth.NewClient(clientOpts)
	orasClient := ORASClient{
		client: c,
	}
	return &orasClient, nil
}

// ORASClientInterface defines the required methods that the acr-cli will need to use with ORAS.
type ORASClientInterface interface {
	Annotate(ctx context.Context, reference string, artifactType string, annotations map[string]string) error
	DiscoverLifecycleAnnotation(ctx context.Context, reference string, artifactType string) (bool, error)
}
