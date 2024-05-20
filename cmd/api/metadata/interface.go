package metadata

import (
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// DiscoverHandler handles metadata output for discover events.
type DiscoverHandler interface {
	// MultiLevelSupported returns true if the handler supports multi-level
	// discovery.
	MultiLevelSupported() bool
	// OnDiscovered is called after a referrer is discovered.
	OnDiscovered(referrer, subject ocispec.Descriptor) error
	// OnCompleted is called when referrer discovery is completed.
	OnCompleted() error
}
