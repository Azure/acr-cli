// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package api

import (
	"context"
	"fmt"

	orasauth "github.com/Azure/acr-cli/auth/oras"
	"oras.land/oras-go/v2/registry/remote"
	// ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	// oras "oras.land/oras-go/v2"
	// // oras "github.com/oras-project/oras-go"
	// content "oras.land/oras-go/v2/content"
	// file "oras.land/oras-go/v2/content/file"
	// "oras.land/oras-go/v2/registry/remote"
	// track "oras.land/oras/cmd/oras/internal/display/status/track"
	// option "oras.land/oras/cmd/oras/internal/option"
	// graph "oras.land/oras/internal/graph"
)

// The ORASClient wraps the oras-go sdk and is used for interacting with artifacts in a registry.
// it implements the ORASClientInterface.
type ORASClient struct {
	client remote.Client
}

func (o *ORASClient) Annotate(ctx context.Context, repoName string, reference string, artifactType string, annotationsArg map[string]string) error {
	// dst, err := o.getTarget(reference)
	// if err != nil {
	// 	return err
	// }

	// // do the equivalent of
	// // prepare manifest
	// store, err := file.New("")
	// if err != nil {
	// 	return err
	// }
	// defer store.Close()

	// subject, err := dst.Resolve(ctx, reference)
	// if err != nil {
	// 	return err
	// }

	// graphCopyOptions := oras.DefaultCopyGraphOptions
	// packOpts := oras.PackManifestOptions{
	// 	Subject:             &subject,
	// 	ManifestAnnotations: annotationsArg[option.AnnotationManifest],
	// }
	// // oras attach --artifact-type <type> ref --annotation k=v
	// pack := func() (ocispec.Descriptor, error) {
	// 	return oras.PackManifest(ctx, store, oras.PackManifestVersion1_1_RC4, artifactType, packOpts)
	// }

	// copy := func(root ocispec.Descriptor) error {
	// 	graphCopyOptions.FindSuccessors = func(ctx context.Context, fetcher content.Fetcher, node ocispec.Descriptor) ([]ocispec.Descriptor, error) {
	// 		if content.Equal(node, root) {
	// 			// skip duplicated Resolve on subject
	// 			successors, _, config, err := graph.Successors(ctx, fetcher, node)
	// 			if err != nil {
	// 				return nil, err
	// 			}
	// 			if config != nil {
	// 				successors = append(successors, *config)
	// 			}
	// 			return successors, nil
	// 		}
	// 		return content.Successors(ctx, fetcher, node)
	// 	}
	// 	return oras.CopyGraph(ctx, store, dst, root, graphCopyOptions)
	// }

	// // Attach
	// _, err = doPush(dst, pack, copy)
	// if err != nil {
	// 	return err
	// }
	// // err = displayMetadata.OnCompleted(&opts.Target, root, subject)
	// // if err != nil {
	// // 	return err
	// // }

	// // // Export manifest
	// // return opts.ExportManifest(ctx, store, root)

	fmt.Printf("Annotating reference: %s", reference)
	return nil
}

// func doPush(dst oras.Target, pack packFunc, copy copyFunc) (ocispec.Descriptor, error) {
// 	if tracked, ok := dst.(track.GraphTarget); ok {
// 		defer tracked.Close()
// 	}
// 	// Push
// 	return pushArtifact(dst, pack, copy)
// }

// type packFunc func() (ocispec.Descriptor, error)
// type copyFunc func(desc ocispec.Descriptor) error

// func pushArtifact(dst oras.Target, pack packFunc, copy copyFunc) (ocispec.Descriptor, error) {
// 	root, err := pack()
// 	if err != nil {
// 		return ocispec.Descriptor{}, err
// 	}

// 	// push
// 	if err = copy(root); err != nil {
// 		return ocispec.Descriptor{}, err
// 	}
// 	return root, nil
// }

// getTarget gets an oras remote.Repository object that refers to the target of our annotation request
func (o *ORASClient) getTarget(reference string) (repo *remote.Repository, err error) {
	repo, err = remote.NewRepository(reference)
	if err != nil {
		// if errors.Unwrap(err) == errdef.ErrInvalidReference {
		// 	return nil, fmt.Errorf("%q: %v", reference, err)
		// }
		return nil, err
	}
	// registry := repo.Reference.Registry
	// //repo.PlainHTTP = opts.isPlainHttp(registry)
	// //repo.HandleWarning = opts.handleWarning(registry, logger)
	// if repo.Client, err = oras.NewClient()
	// // if repo.Client, err = opts.authClient(registry, common.Debug); err != nil {
	// // 	return nil, err
	// // }
	repo.SkipReferrersGC = true
	repo.Client = o.client // remote repo reference w/ client set on top
	repo.SetReferrersCapability(true)
	return repo, nil
}

func GetORASClientWithAuth(loginURL string, username string, password string, configs []string) (*ORASClient, error) {
	// 	opts := orasauth.ClientOptions{}
	// 	client := orasauth.NewClient(opts)
	// }
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
	Annotate(ctx context.Context, repoName string, reference string, artifactType string, annotations map[string]string) error
	// GetAcrTags(ctx context.Context, repoName string, orderBy string, last string) (*acrapi.RepositoryTagsType, error)
	// DeleteAcrTag(ctx context.Context, repoName string, reference string) (*autorest.Response, error)
	// GetAcrManifests(ctx context.Context, repoName string, orderBy string, last string) (*acrapi.Manifests, error)
	// DeleteManifest(ctx context.Context, repoName string, reference string) (*autorest.Response, error)
	// GetManifest(ctx context.Context, repoName string, reference string) ([]byte, error)
}
