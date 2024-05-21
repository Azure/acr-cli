package model

import ocispec "github.com/opencontainers/image-spec/specs-go/v1"

// Descriptor is a descriptor with digest reference.
// We cannot use ocispec.Descriptor here since the first letter of the json
// annotation key is not uppercase.
type Descriptor struct {
	DigestReference
	ocispec.Descriptor
}

// DigestReference is a reference to an artifact with digest.
type DigestReference struct {
	Reference string `json:"reference"`
}

// NewDigestReference creates a new digest reference.
func NewDigestReference(name string, digest string) DigestReference {
	return DigestReference{
		Reference: name + "@" + digest,
	}
}

// FromDescriptor converts a OCI descriptor to a descriptor with digest reference.
func FromDescriptor(name string, desc ocispec.Descriptor) Descriptor {
	ret := Descriptor{
		DigestReference: NewDigestReference(name, desc.Digest.String()),
		Descriptor: ocispec.Descriptor{
			MediaType:    desc.MediaType,
			Size:         desc.Size,
			Digest:       desc.Digest,
			Annotations:  desc.Annotations,
			ArtifactType: desc.ArtifactType,
		},
	}
	return ret
}
