package option

import (
	"os"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// Common option struct.
type Common struct {
	Debug   bool
	Verbose bool
	TTY     *os.File
}

// Platform option struct.
type Platform struct {
	Platform        *ocispec.Platform
	FlagDescription string
}
