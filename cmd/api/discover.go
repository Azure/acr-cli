package api

import (
	"errors"
	"fmt"

	"github.com/Azure/acr-cli/cmd/api/argument"
	"github.com/Azure/acr-cli/cmd/api/command"
	"github.com/Azure/acr-cli/cmd/api/display"
	oerrors "github.com/Azure/acr-cli/cmd/api/errors"
	"github.com/Azure/acr-cli/cmd/api/option"
	"github.com/spf13/cobra"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/registry"
)

type DiscoverOptions struct {
	option.Common
	option.Platform
	option.Target
	option.Format

	artifactType string
}

func useDiscoverCmd() *cobra.Command {
	var opts DiscoverOptions
	cmd := &cobra.Command{
		Use:   "discover [flags] <name>{:<tag>|@<digest>}",
		Short: "[Preview] Discover referrers of a manifest in a registry or an OCI image layout",
		Long: `[Preview] Discover referrers of a manifest in a registry or an OCI image layout

** This command is in preview and under development. **

Example - Discover direct referrers of manifest 'hello:v1' in registry 'localhost:5000':
  oras discover localhost:5000/hello:v1

Example - Discover direct referrers via referrers API:
  oras discover --distribution-spec v1.1-referrers-api localhost:5000/hello:v1

Example - Discover direct referrers via tag scheme:
  oras discover --distribution-spec v1.1-referrers-tag localhost:5000/hello:v1

Example - Discover all the referrers of manifest 'hello:v1' in registry 'localhost:5000', displayed in a tree view:
  oras discover -o tree localhost:5000/hello:v1

Example - Discover all the referrers of manifest with annotations, displayed in a tree view:
  oras discover -v -o tree localhost:5000/hello:v1

Example - Discover referrers with type 'test-artifact' of manifest 'hello:v1' in registry 'localhost:5000':
  oras discover --artifact-type test-artifact localhost:5000/hello:v1

Example - Discover referrers of the manifest tagged 'v1' in an OCI image layout folder 'layout-dir':
  oras discover --oci-layout layout-dir:v1
  oras discover --oci-layout -v -o tree layout-dir:v1
`,
		Args: oerrors.CheckArgs(argument.Exactly(1), "the target artifact to discover referrers from"),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if err := oerrors.CheckMutuallyExclusiveFlags(cmd.Flags(), "format", "output"); err != nil {
				return err
			}
			opts.RawReference = args[0]
			if err := option.Parse(cmd, &opts); err != nil {
				return err
			}
			if cmd.Flags().Changed("output") {
				switch opts.Format.Type {
				case "tree", "json", "table":
					fmt.Fprintf(cmd.ErrOrStderr(), "[DEPRECATED] --output is deprecated, try `--format %s` instead\n", opts.Template)
				default:
					fmt.Printf("format type = %s", opts.Format.Type)
					return errors.New("output type can only be tree, table or json")
				}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := RunDiscover(cmd, &opts)
			return err
		},
	}

	cmd.Flags().StringVarP(&opts.artifactType, "artifact-type", "", "", "artifact type")
	cmd.Flags().StringVarP(&opts.Format.FormatFlag, "output", "o", "tree", "[Deprecated] format in which to display referrers (table, json, or tree). tree format will also show indirect referrers")
	opts.FormatFlag = option.FormatTypeTree.Name
	opts.AllowedTypes = []*option.FormatType{
		option.FormatTypeTree,
		option.FormatTypeTable,
		option.FormatTypeJSON.WithUsage("Get direct referrers and output in JSON format"),
		option.FormatTypeGoTemplate.WithUsage("Print direct referrers using the given Go template"),
	}
	opts.EnableDistributionSpecFlag()
	option.ApplyFlags(&opts, cmd.Flags())
	return oerrors.Command(cmd, &opts.Target)
}

func RunDiscover(cmd *cobra.Command, opts *DiscoverOptions) (bool, error) {
	skipAnnotate := false
	// fmt.Println("MADE IT")
	ctx, logger := command.GetLogger(cmd, &opts.Common)
	// fmt.Println("MADE IT 0.5")
	repo, err := opts.NewReadonlyTarget(ctx, opts.Common, logger)
	if err != nil {
		// fmt.Println("ERR TRUE")
		return true, err
	}
	// fmt.Println("MADE IT 1")
	resolveOpts := oras.DefaultResolveOptions
	desc, err := oras.Resolve(ctx, repo, opts.Reference, resolveOpts)
	if err != nil {
		return true, err
	}
	// fmt.Println("MADE IT 2")
	_, err = display.NewDiscoverHandler(cmd.OutOrStdout(), opts.Format, opts.Path, opts.RawReference, desc, opts.Verbose)
	if err != nil {
		return true, err
	}
	// fmt.Println("MADE IT 3")
	refs, err := registry.Referrers(ctx, repo, desc, opts.artifactType)
	if err != nil {
		return true, err
	}
	// fmt.Println("MADE IT")
	// fmt.Printf("refs length = %d\n", len(refs))
	for _, ref := range refs {
		// if err := handler.OnDiscovered(ref, desc); err != nil {
		// 	return true, err
		// } else
		// fmt.Printf("ref digest = %s\n", ref.Digest)
		if ref.ArtifactType == "application/vnd.microsoft.artifact.lifecycle" {
			// fmt.Printf("ref = %s\n", ref.ArtifactType)
			skipAnnotate = true
			// fmt.Printf("skipAnnotate = %t\n", skipAnnotate)
			// If the artifact type is a lifecycle annotation one, do not annotate the image any further
		}
	}

	// fmt.Println("BEFORE")
	// err = handler.OnCompleted()
	// fmt.Println("MADE IT")

	return skipAnnotate, err
}
