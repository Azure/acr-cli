package model

import ocispec "github.com/opencontainers/image-spec/specs-go/v1"

type discover struct {
	Manifests []Descriptor `json:"manifests"`
}

// NewDiscover creates a new discover model.
func NewDiscover(name string, descs []ocispec.Descriptor) discover {
	discover := discover{
		Manifests: make([]Descriptor, 0, len(descs)),
	}
	for _, desc := range descs {
		discover.Manifests = append(discover.Manifests, FromDescriptor(name, desc))
	}
	return discover
}
