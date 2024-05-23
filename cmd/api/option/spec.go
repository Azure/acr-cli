package option

import (
	"fmt"
	"strings"

	oerrors "github.com/Azure/acr-cli/cmd/api/errors"
	"github.com/spf13/pflag"
)

const (
	DistributionSpecReferrersTagV1_1 = "v1.1-referrers-tag"
	DistributionSpecReferrersAPIV1_1 = "v1.1-referrers-api"
)

// DistributionSpec option struct which implements pflag.Value interface.
type DistributionSpec struct {
	// ReferrersAPI indicates the preference of the implementation of the Referrers API.
	// Set to true for referrers API, false for referrers tag scheme, and nil for auto fallback.
	ReferrersAPI *bool
	// specFlag should be provided in form of`<version>-<api>-<option>`
	flag string
}

// Set validates and sets the flag value from a string argument.
func (ds *DistributionSpec) Set(value string) error {
	ds.flag = value
	switch ds.flag {
	case DistributionSpecReferrersTagV1_1:
		isApi := false
		ds.ReferrersAPI = &isApi
	case DistributionSpecReferrersAPIV1_1:
		isApi := true
		ds.ReferrersAPI = &isApi
	default:
		return &oerrors.Error{
			Err:            fmt.Errorf("unknown distribution specification flag: %s", value),
			Recommendation: fmt.Sprintf("Available options: %s", ds.Options()),
		}
	}
	return nil
}

// ApplyFlagsWithPrefix applies flags to a command flag set with a prefix string.
func (ds *DistributionSpec) ApplyFlagsWithPrefix(fs *pflag.FlagSet, prefix, description string) {
	flagPrefix, notePrefix := applyPrefix(prefix, description)
	fs.Var(ds, flagPrefix+"distribution-spec", fmt.Sprintf("[Preview] set OCI distribution spec version and API option for %starget. Options: %s", notePrefix, ds.Options()))
}

// Options returns the string of usable options for the flag.
func (ds *DistributionSpec) Options() string {
	return strings.Join([]string{
		DistributionSpecReferrersTagV1_1,
		DistributionSpecReferrersAPIV1_1,
	}, ", ")
}

// String returns the string representation of the flag.
func (ds *DistributionSpec) String() string {
	return ds.flag
}

// Type returns the string value of the inner flag.
func (ds *DistributionSpec) Type() string {
	return "string"
}
