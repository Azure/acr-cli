package display

import (
	"io"

	"github.com/Azure/acr-cli/cmd/api/errors"
	"github.com/Azure/acr-cli/cmd/api/metadata"
	"github.com/Azure/acr-cli/cmd/api/metadata/json"
	"github.com/Azure/acr-cli/cmd/api/metadata/table"
	"github.com/Azure/acr-cli/cmd/api/metadata/template"
	"github.com/Azure/acr-cli/cmd/api/metadata/tree"
	"github.com/Azure/acr-cli/cmd/api/option"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// NewDiscoverHandler returns status and metadata handlers for discover command.
func NewDiscoverHandler(out io.Writer, format option.Format, path string, rawReference string, desc ocispec.Descriptor, verbose bool) (metadata.DiscoverHandler, error) {
	var handler metadata.DiscoverHandler
	switch format.Type {
	case option.FormatTypeTree.Name, "":
		handler = tree.NewDiscoverHandler(out, path, desc, verbose)
	case option.FormatTypeTable.Name:
		handler = table.NewDiscoverHandler(out, rawReference, desc, verbose)
	case option.FormatTypeJSON.Name:
		handler = json.NewDiscoverHandler(out, desc, path)
	case option.FormatTypeGoTemplate.Name:
		handler = template.NewDiscoverHandler(out, desc, path, format.Template)
	default:
		return nil, errors.UnsupportedFormatTypeError(format.Type)
	}
	return handler, nil
}
