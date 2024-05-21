package option

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"

	"github.com/sirupsen/logrus"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/oci"
	"oras.land/oras-go/v2/errdef"
	"oras.land/oras-go/v2/registry"
	"oras.land/oras-go/v2/registry/remote"
)

const (
	TargetTypeRemote    = "registry"
	TargetTypeOCILayout = "oci-layout"
)

// ReadOnlyGraphTagFinderTarget represents a read-only graph target with tag
// finder capability.
type ReadOnlyGraphTagFinderTarget interface {
	oras.ReadOnlyGraphTarget
	registry.TagLister
}

// Target struct contains flags and arguments specifying one registry or image
// layout.
// Target implements oerrors.Handler interface.
type Target struct {
	Remote
	RawReference string
	Type         string
	Reference    string //contains tag or digest
	// Path contains
	//  - path to the OCI image layout target, or
	//  - registry and repository for the remote target
	Path string

	IsOCILayout bool
}

// NewReadonlyTargets generates a new read only target based on opts.
func (opts *Target) NewReadonlyTarget(ctx context.Context, common Common, logger logrus.FieldLogger) (ReadOnlyGraphTagFinderTarget, error) {
	switch opts.Type {
	case TargetTypeOCILayout:
		info, err := os.Stat(opts.Path)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil, fmt.Errorf("invalid argument %q: failed to find path %q: %w", opts.RawReference, opts.Path, err)
			}
			return nil, err
		}
		if info.IsDir() {
			return oci.NewFromFS(ctx, os.DirFS(opts.Path))
		}
		store, err := oci.NewFromTar(ctx, opts.Path)
		if err != nil {
			if errors.Is(err, io.ErrUnexpectedEOF) {
				return nil, fmt.Errorf("%q does not look like a tar archive: %w", opts.Path, err)
			}
			return nil, err
		}
		return store, nil
	case TargetTypeRemote:
		repo, err := opts.NewRepository(opts.RawReference, common, logger)
		if err != nil {
			return nil, err
		}
		tmp := repo.Reference
		tmp.Reference = ""
		opts.Path = tmp.String()
		opts.Reference = repo.Reference.Reference
		return repo, nil
	}
	return nil, fmt.Errorf("unknown target type: %q", opts.Type)
}

// NewRepository assembles a oras remote repository.
func (opts *Remote) NewRepository(reference string, common Common, logger logrus.FieldLogger) (repo *remote.Repository, err error) {
	repo, err = remote.NewRepository(reference)
	if err != nil {
		if errors.Unwrap(err) == errdef.ErrInvalidReference {
			return nil, fmt.Errorf("%q: %v", reference, err)
		}
		return nil, err
	}
	registry := repo.Reference.Registry
	repo.PlainHTTP = opts.isPlainHttp(registry)
	repo.HandleWarning = opts.handleWarning(registry, logger)
	if repo.Client, err = opts.authClient(common.Debug); err != nil {
		return nil, err
	}
	repo.SkipReferrersGC = true
	if opts.ReferrersAPI != nil {
		if err := repo.SetReferrersCapability(*opts.ReferrersAPI); err != nil {
			return nil, err
		}
	}
	return
}
