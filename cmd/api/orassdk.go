// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package api

import (
	"context"
	"fmt"
	"net/http"

	"github.com/pkg/errors"
	orasauth "github.com/Azure/acr-cli/auth/oras"
	"oras.land/oras-go/v2/errdef"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/retry"
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


func (o *ORASClient) GetORASClientWithAuth(loginURL string, username string, password string, configs []string) (*ORASClient, error) {
// 	opts := orasauth.ClientOptions{}
// 	client := orasauth.NewClient(opts)
// }
	if username == "" && password == "" {
		store, err := oras.NewStore(configs...)
		if err != nil {
			return nil, errors.Wrap(err, "error resolving authentication")
		}
		cred, err := store.Credential(context.Background(), loginURL)
		if err != nil {
			return nil, errors.Wrap(err, "error resolving authentication")
		}
		username = cred.Username
		password = cred.Password

		// fallback to refresh token if it is available
		if password == "" && cred.RefreshToken != "" {
			password = cred.RefreshToken
		}
	}
	// If the password is empty then the authentication failed.
	if password == "" {
		return nil, errors.New("unable to resolve authentication, missing identity token or password")
	}
	if username == "" || username == "00000000-0000-0000-0000-000000000000" {
		// // If the username is empty an ACR refresh token was used.
		// var err error
		// acrClient, err = newAcrCLIClientWithBearerAuth(loginURL, password)
		// if err != nil {
		// 	return nil, errors.Wrap(err, "error resolving authentication")
		// }
		// return &acrClient, nil
		return nil, nil
	}

	// if we made it here we are logging in with username and password
	cred := orasauth.Credential(username, password)
	c := orasauth.NewClient(oras.ClientOptions{
		Credential: cred,
		Debug:      opts.debug,
	})
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
