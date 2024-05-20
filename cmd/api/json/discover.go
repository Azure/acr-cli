package json

import (
	"io"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// discoverHandler handles json metadata output for discover events.
type discoverHandler struct {
	out       io.Writer
	root      ocispec.Descriptor
	path      string
	referrers []ocispec.Descriptor
}
