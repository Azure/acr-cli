package tree

import (
	"fmt"
	"io"

	"github.com/Azure/acr-cli/cmd/api/metadata"
	"github.com/Azure/acr-cli/cmd/api/tree"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// discoverHandler handles json metadata output for discover events.
type discoverHandler struct {
	out     io.Writer
	path    string
	root    *tree.Node
	nodes   map[digest.Digest]*tree.Node
	verbose bool
}

// NewDiscoverHandler creates a new handler for discover events.
func NewDiscoverHandler(out io.Writer, path string, root ocispec.Descriptor, verbose bool) metadata.DiscoverHandler {
	treeRoot := tree.New(fmt.Sprintf("%s@%s", path, root.Digest))
	return &discoverHandler{
		out:  out,
		path: path,
		root: treeRoot,
		nodes: map[digest.Digest]*tree.Node{
			root.Digest: treeRoot,
		},
		verbose: verbose,
	}
}

// OnDiscovered implements metadata.DiscoverHandler.
// func (h *discoverHandler) OnDiscovered(referrer, subject ocispec.Descriptor) error {
// 	node, ok := h.nodes[subject.Digest]
// 	if !ok {
// 		return fmt.Errorf("unexpected subject descriptor: %v", subject)
// 	}
// 	if referrer.ArtifactType == "" {
// 		referrer.ArtifactType = "<unknown>"
// 	}
// 	referrerNode := node.AddPath(referrer.ArtifactType, referrer.Digest)
// 	if h.verbose {
// 		for k, v := range referrer.Annotations {
// 			bytes, err := yaml.Marshal(map[string]string{k: v})
// 			if err != nil {
// 				return err
// 			}
// 			referrerNode.AddPath(strings.TrimSpace(string(bytes)))
// 		}
// 	}
// 	h.nodes[referrer.Digest] = referrerNode
// 	return nil
// }

// // OnCompleted implements metadata.DiscoverHandler.
// func (h *discoverHandler) OnCompleted() error {
// 	// return tree.NewPrinter(h.out).Print(h.root)
// 	return nil
// }
