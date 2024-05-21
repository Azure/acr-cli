package json

import (
	"fmt"
	"io"

	"github.com/Azure/acr-cli/cmd/api/display/model"
	"github.com/Azure/acr-cli/cmd/api/display/utils"
	"github.com/Azure/acr-cli/cmd/api/metadata"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/content"
)

// discoverHandler handles json metadata output for discover events.
type discoverHandler struct {
	out       io.Writer
	root      ocispec.Descriptor
	path      string
	referrers []ocispec.Descriptor
}

// NewDiscoverHandler creates a new handler for discover events.
func NewDiscoverHandler(out io.Writer, root ocispec.Descriptor, path string) metadata.DiscoverHandler {
	return &discoverHandler{
		out:  out,
		root: root,
		path: path,
	}
}

// OnDiscovered implements metadata.DiscoverHandler.
func (h *discoverHandler) OnDiscovered(referrer, subject ocispec.Descriptor) error {
	if !content.Equal(subject, h.root) {
		return fmt.Errorf("unexpected subject descriptor: %v", subject)
	}
	h.referrers = append(h.referrers, referrer)
	return nil
}

// OnCompleted implements metadata.DiscoverHandler.
func (h *discoverHandler) OnCompleted() error {
	return utils.PrintPrettyJSON(h.out, model.NewDiscover(h.path, h.referrers))
}
