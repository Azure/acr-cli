// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package api

import (
	"context"
	"io"

	orasauth "github.com/Azure/acr-cli/auth/oras"
	"github.com/Azure/acr-cli/cmd/api/command"
	"github.com/Azure/acr-cli/cmd/api/option"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/spf13/cobra"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/file"
	"oras.land/oras-go/v2/registry"
	"oras.land/oras-go/v2/registry/remote"
)

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

type DiscoverOptions struct {
	option.Common
	option.Platform
	option.Target
	// option.Format

	artifactType string
}

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
	_, err = doPush(dst, pack, copyFunc)
	if err != nil {
		return err
	}

	return nil
}

func doPush(dst oras.Target, pack packFunc, copy copyFunc) (ocispec.Descriptor, error) {
	if tracked, ok := dst.(GraphTarget); ok {
		defer tracked.Close()
	}
	// Push
	return pushArtifact(pack, copy)
}

type packFunc func() (ocispec.Descriptor, error)
type copyFunc func(desc ocispec.Descriptor) error

func pushArtifact(pack packFunc, copy copyFunc) (ocispec.Descriptor, error) {
	root, err := pack()
	if err != nil {
		return ocispec.Descriptor{}, err
	}

	// push
	if err = copy(root); err != nil {
		return ocispec.Descriptor{}, err
	}
	return root, nil
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

func RunDiscover(cmd *cobra.Command, opts *DiscoverOptions) (bool, error) {
	skipAnnotate := false
	ctx, logger := command.GetLogger(cmd, &opts.Common)
	repo, err := opts.NewReadonlyTarget(ctx, opts.Common, logger)
	if err != nil {
		return true, err
	}
	resolveOpts := oras.DefaultResolveOptions
	desc, err := oras.Resolve(ctx, repo, opts.Reference, resolveOpts)
	if err != nil {
		return true, err
	}
	refs, err := registry.Referrers(ctx, repo, desc, opts.artifactType)
	if err != nil {
		return true, err
	}

	for _, ref := range refs {

		if ref.ArtifactType == "application/vnd.microsoft.artifact.lifecycle" {
			skipAnnotate = true
		}
	}
	return skipAnnotate, err
}

// ORASClientInterface defines the required methods that the acr-cli will need to use with ORAS.
type ORASClientInterface interface {
	Annotate(ctx context.Context, reference string, artifactType string, annotations map[string]string) error
}
