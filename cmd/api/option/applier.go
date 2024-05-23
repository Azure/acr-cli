package option

import (
	"github.com/spf13/pflag"
)

// FlagApplier applies flags to a command flag set.
type FlagApplier interface {
	ApplyFlags(*pflag.FlagSet)
}

// ApplyFlags applies applicable fields of the passed-in option pointer to the
// target flag set.
// NOTE: The option argument need to be a pointer to the options, so its value
// becomes addressable.
func ApplyFlags(optsPtr interface{}, target *pflag.FlagSet) {
	_ = rangeFields(optsPtr, func(fa FlagApplier) error {
		fa.ApplyFlags(target)
		return nil
	})
}
