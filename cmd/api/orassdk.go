// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package api

import (
	"context"

	"github.com/Azure/acr-cli/auth/oras"
	orasauth "github.com/Azure/acr-cli/auth/oras"
	"oras.land/oras-go/v2/registry/remote"
)

// The ORASClient wraps the oras-go sdk and is used for interacting with artifacts in a registry.
// it implements the ORASClientInterface.
type ORASClient struct {
	client remote.Client
}

func (o *ORASClient) Annotate(ctx context.Context, repoName string, reference string, artifactType string, annotationsArg map[string]string) error {
	// do the equivalent of
	// // prepare manifest
	// store, err := file.New("")
	// if err != nil {
	//	return err
	// }
	//defer store.Close()
	// graphCopyOptions := oras.DefaultCopyGraphOptions
	// packOpts := oras.PackManifestOptions{
	//	Subject:             &subject,
	//	ManifestAnnotations: annotations[option.AnnotationManifest],
	//
	//}
	// oras attach --artifact-type <type> ref --annotation k=v
	// oras.PackManifest(ctx, store, oras.PackManifestVersion1_1_RC4, opts.artifactType, packOpts)
	// graphCopyOptions.FindSuccessors = func(ctx context.Context, fetcher content.Fetcher, node ocispec.Descriptor) ([]ocispec.Descriptor, error) {
	// 	if content.Equal(node, root) {
	// 		// skip duplicated Resolve on subject
	// 		successors, _, config, err := graph.Successors(ctx, fetcher, node)
	// 		if err != nil {
	// 			return nil, err
	// 		}
	// 		if config != nil {
	// 			successors = append(successors, *config)
	// 		}
	// 		return successors, nil
	// 	}
	// 	return content.Successors(ctx, fetcher, node)
	// }
	// oras.CopyGraph(ctx, store, dst, root, graphCopyOptions)
	return nil
}

// getTarget gets an oras remote.Repository object that refers to the target of our annotation request
func getTarget(reference string) (repo *remote.Repository, err error) {
	// repo, err = remote.NewRepository(reference)
	// if err != nil {
	// 	if errors.Unwrap(err) == errdef.ErrInvalidReference {
	// 		return nil, fmt.Errorf("%q: %v", reference, err)
	// 	}
	// 	return nil, err
	// }
	// registry := repo.Reference.Registry
	// //repo.PlainHTTP = opts.isPlainHttp(registry)
	// //repo.HandleWarning = opts.handleWarning(registry, logger)
	// if repo.Client, err = oras.NewClient()
	// // if repo.Client, err = opts.authClient(registry, common.Debug); err != nil {
	// // 	return nil, err
	// // }
	// repo.SkipReferrersGC = true
	// repo.SetReferrersCapability(true)
	// return repo, nil
	return nil, nil
}

func GetORASClientWithAuth(loginURL string, username string, password string, configs []string) (*ORASClient, error) {
	// 	opts := orasauth.ClientOptions{}
	// 	client := orasauth.NewClient(opts)
	// }
	clientOpts := oras.ClientOptions{}
	if username != "" && password != "" {
		clientOpts.Credential = orasauth.Credential(username, password)
	} else {
		store, err := oras.NewStore(configs...)
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
