// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	orasauth "github.com/Azure/acr-cli/auth/oras"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/registry/remote"

	// // oras "github.com/oras-project/oras-go"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/content/file"
	// track "oras.land/oras/cmd/oras/internal/display/status/track"
	// option "oras.land/oras/cmd/oras/internal/option"
	// "oras.land/oras/internal/graph"
)

const (
	DockerMediaTypeManifest = "application/vnd.docker.distribution.manifest.v2+json"
	// MediaTypeImageManifest specifies the media type for an image manifest.
	MediaTypeImageManifest = "application/vnd.oci.image.manifest.v1+json"
	// MediaTypeArtifactManifest specifies the media type for a content descriptor.
	MediaTypeArtifactManifest = "application/vnd.oci.artifact.manifest.v1+json"
)

// The ORASClient wraps the oras-go sdk and is used for interacting with artifacts in a registry.
// it implements the ORASClientInterface.
type ORASClient struct {
	client remote.Client
}

// Artifact describes an artifact manifest.
// This structure provides `application/vnd.oci.artifact.manifest.v1+json` mediatype when marshalled to JSON.
//
// This manifest type was introduced in image-spec v1.1.0-rc1 and was removed in
// image-spec v1.1.0-rc3. It is not part of the current image-spec and is kept
// here for Go compatibility.
//
// Reference: https://github.com/opencontainers/image-spec/pull/999
type Artifact struct {
	// MediaType is the media type of the object this schema refers to.
	MediaType string `json:"mediaType"`

	// ArtifactType is the IANA media type of the artifact this schema refers to.
	ArtifactType string `json:"artifactType"`

	// Blobs is a collection of blobs referenced by this manifest.
	Blobs []ocispec.Descriptor `json:"blobs,omitempty"`

	// Subject (reference) is an optional link from the artifact to another manifest forming an association between the artifact and the other manifest.
	Subject *ocispec.Descriptor `json:"subject,omitempty"`

	// Annotations contains arbitrary metadata for the artifact manifest.
	Annotations map[string]string `json:"annotations,omitempty"`
}

// GraphTarget is a tracked oras.GraphTarget.
type GraphTarget interface {
	oras.GraphTarget
	io.Closer
	Prompt(desc ocispec.Descriptor, prompt string) error
	Inner() oras.GraphTarget
}

func (o *ORASClient) Annotate(ctx context.Context, repoName string, reference string, artifactType string, annotationsArg map[string]string) error {
	// fmt.Println("registry: ", o.r)
	fmt.Println("reference: ", reference)

	// ref := fmt.Sprintf("%s/%s:%s", registry, repository, reference)
	// 	if strings.HasPrefix(reference, "sha256") {
	// 		ref = fmt.Sprintf("%s/%s@%s", registry, repository, reference)
	// 	}

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
		ManifestAnnotations: annotationsArg, // map of annotations
	}
	// oras attach --artifact-type <type> ref --annotation k=v
	pack := func() (ocispec.Descriptor, error) {
		return oras.PackManifest(ctx, store, oras.PackManifestVersion1_1_RC4, artifactType, packOpts)
	}

	copy := func(root ocispec.Descriptor) error {
		graphCopyOptions.FindSuccessors = func(ctx context.Context, fetcher content.Fetcher, node ocispec.Descriptor) ([]ocispec.Descriptor, error) {
			if content.Equal(node, root) {
				// skip duplicated Resolve on subject
				// successors, _, config, err := graph.Successors(ctx, fetcher, node)
				// if err != nil {
				// 	return nil, err
				// }

				var nodes []ocispec.Descriptor
				var config *ocispec.Descriptor
				switch node.MediaType {
				case DockerMediaTypeManifest, MediaTypeImageManifest:
					var fetched []byte
					fetched, err = content.FetchAll(ctx, fetcher, node)
					if err != nil {
						return nil, err
					}
					var manifest ocispec.Manifest
					if err = json.Unmarshal(fetched, &manifest); err != nil {
						return nil, err
					}
					nodes = manifest.Layers
					config = &manifest.Config
				case MediaTypeArtifactManifest:
					var fetched []byte
					fetched, err = content.FetchAll(ctx, fetcher, node)
					if err != nil {
						return nil, err
					}
					var manifest Artifact
					if err = json.Unmarshal(fetched, &manifest); err != nil {
						return nil, err
					}
					nodes = manifest.Blobs
				case ocispec.MediaTypeImageIndex:
					var fetched []byte
					fetched, err = content.FetchAll(ctx, fetcher, node)
					if err != nil {
						return nil, err
					}
					var index ocispec.Index
					if err = json.Unmarshal(fetched, &index); err != nil {
						return nil, err
					}
					nodes = index.Manifests
				default:
					nodes, err = content.Successors(ctx, fetcher, node)
				}

				if config != nil {
					nodes = append(nodes, *config)
				}
				return nodes, nil
			}
			return content.Successors(ctx, fetcher, node)
		}
		return oras.CopyGraph(ctx, store, dst, root, graphCopyOptions)
	}

	// Attach
	_, err = doPush(dst, pack, copy) // pack is thing you push
	if err != nil {
		return err
	}
	// err = displayMetadata.OnCompleted(&opts.Target, root, subject)
	// if err != nil {
	// 	return err
	// }

	// // Export manifest
	// return opts.ExportManifest(ctx, store, root)

	fmt.Printf("Annotating reference: %s", reference)
	return nil
}

func doPush(dst oras.Target, pack packFunc, copy copyFunc) (ocispec.Descriptor, error) {
	if tracked, ok := dst.(GraphTarget); ok {
		defer tracked.Close()
	}
	// Push
	return pushArtifact(dst, pack, copy)
}

type packFunc func() (ocispec.Descriptor, error)
type copyFunc func(desc ocispec.Descriptor) error

func pushArtifact(dst oras.Target, pack packFunc, copy copyFunc) (ocispec.Descriptor, error) {
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
