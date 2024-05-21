package option

import (
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// Platform option struct.
type Platform struct {
	Platform        *ocispec.Platform
	FlagDescription string
}
